// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	auth_model "forgejo.org/models/auth"
	"forgejo.org/models/perm"
	"forgejo.org/models/unittest"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/private"
	"forgejo.org/modules/setting"
	api "forgejo.org/modules/structs"
	"forgejo.org/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const botTestSSHKey = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC4cn+iXnA4KvcQYSV88vGn0Yi91vG47t1P7okprVmhNTkipNRIHWr6WdCO4VDr/cvsRkuVJAsLO2enwjGWWueOO6BodiBgyAOZ/5t5nJNMCNuLGT5UIo/RI1b0WRQwxEZTRjt6mFNw6lH14wRd8ulsr9toSWBPMOGWoYs1PDeDL0JuTjL+tr1SZi/EyxCngpYszKdXllJEHyI79KQgeD0Vt3pTrkbNVTOEcCNqZePSVmUH8X8Vhugz3bnE0/iE9Pb5fkWO9c4AnM1FgI/8Bvp27Fw2ShryIXuR6kKvUqhVMTuOSDHwu6A8jLE5Owt3GAYugDpDYuwTVNGrHLXKpPzrGGPE/jPmaLCMZcsdkec95dYeU3zKODEm8UQZFhmJmDeWVJ36nGrGZHL4J5aTTaeFUJmmXDaJYiJ+K2/ioKgXqnXvltu0A9R8/LGy4nrTJRr4JMLuJFoUXvGm1gXQ70w2LSpk6yl71RNC0hCtsBe8BP8IhYCM0EP5jh7eCMQZNvM= bot-key"

func TestAPIAdminCreateBotUserValidation(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminToken := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteAdmin)

	for _, tc := range []struct {
		name   string
		option api.CreateUserOption
	}{
		{"password", api.CreateUserOption{Username: "bad-bot", UserType: "bot", Password: "some-password-123"}},
		{"must_change_password", api.CreateUserOption{Username: "bad-bot", UserType: "bot", MustChangePassword: new(true)}},
		{"login_name", api.CreateUserOption{Username: "bad-bot", UserType: "bot", LoginName: "bad-bot"}},
		{"source_id", api.CreateUserOption{Username: "bad-bot", UserType: "bot", SourceID: 1}},
		{"unknown user_type", api.CreateUserOption{Username: "bad-bot", Email: "bad-bot@example.com", UserType: "robot"}},
		{"individual without email", api.CreateUserOption{Username: "no-email", Password: "some-password-123"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := NewRequestWithJSON(t, "POST", "/api/v1/admin/users", tc.option).AddTokenAuth(adminToken)
			MakeRequest(t, req, http.StatusUnprocessableEntity)
			unittest.AssertNotExistsBean(t, &user_model.User{Name: tc.option.Username})
		})
	}

	t.Run("non-admin cannot create bots", func(t *testing.T) {
		userToken := getUserToken(t, "user2", auth_model.AccessTokenScopeWriteAdmin)
		req := NewRequestWithJSON(t, "POST", "/api/v1/admin/users", api.CreateUserOption{Username: "sneaky-bot", UserType: "bot"}).
			AddTokenAuth(userToken)
		MakeRequest(t, req, http.StatusForbidden)
		unittest.AssertNotExistsBean(t, &user_model.User{Name: "sneaky-bot"})
	})
}

func TestAPIAdminBotUser(t *testing.T) {
	onApplicationRun(t, func(t *testing.T, _ *url.URL) {
		adminToken := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteAdmin)
		const botName = "deploy-bot"

		// Create the bot without password or email.
		req := NewRequestWithJSON(t, "POST", "/api/v1/admin/users", api.CreateUserOption{
			Username:   botName,
			UserType:   "bot",
			SendNotify: true,
		}).AddTokenAuth(adminToken)
		resp := MakeRequest(t, req, http.StatusCreated)
		var apiBot api.User
		DecodeJSON(t, resp, &apiBot)
		assert.True(t, apiBot.IsBot)
		assert.Equal(t, botName+"@"+setting.Service.NoReplyAddress, apiBot.Email)

		bot := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: botName})
		assert.Equal(t, user_model.UserTypeBot, bot.Type)
		assert.False(t, bot.IsPasswordSet())
		assert.False(t, bot.MustChangePassword)
		assert.True(t, bot.IsActive)

		// A bot is publicly recognisable as such.
		resp = MakeRequest(t, NewRequest(t, "GET", "/api/v1/users/"+botName), http.StatusOK)
		var publicBot api.User
		DecodeJSON(t, resp, &publicBot)
		assert.True(t, publicBot.IsBot)

		// Give the bot a private repository, an access token and an SSH key.
		req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/admin/users/%s/repos", botName), api.CreateRepoOption{
			Name:     "private-repo",
			Private:  true,
			AutoInit: true,
		}).AddTokenAuth(adminToken)
		MakeRequest(t, req, http.StatusCreated)

		req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/admin/users/%s/tokens", botName), api.CreateAccessTokenOption{
			Name:   "ci",
			Scopes: []string{"read:user", "read:repository"},
		}).AddTokenAuth(adminToken)
		resp = MakeRequest(t, req, http.StatusCreated)
		var botToken api.AccessToken
		DecodeJSON(t, resp, &botToken)

		req = NewRequestWithValues(t, "POST", fmt.Sprintf("/api/v1/admin/users/%s/keys", botName), map[string]string{
			"key":   botTestSSHKey,
			"title": "bot-key",
		}).AddTokenAuth(adminToken)
		resp = MakeRequest(t, req, http.StatusCreated)
		var botKey api.PublicKey
		DecodeJSON(t, resp, &botKey)

		t.Run("token authenticates against the API", func(t *testing.T) {
			resp := MakeRequest(t, NewRequest(t, "GET", "/api/v1/user").AddTokenAuth(botToken.Token), http.StatusOK)
			var self api.User
			DecodeJSON(t, resp, &self)
			assert.Equal(t, botName, self.UserName)
		})

		t.Run("token authenticates git over HTTP", func(t *testing.T) {
			refs := fmt.Sprintf("/%s/private-repo.git/info/refs", botName)
			MakeRequest(t, NewRequest(t, "GET", refs), http.StatusUnauthorized)

			req := NewRequest(t, "GET", refs)
			req.SetBasicAuth(botName, botToken.Token)
			MakeRequest(t, req, http.StatusOK)
		})

		t.Run("SSH key authenticates git over SSH", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			results, extra := private.ServCommand(ctx, botKey.ID, botName, "private-repo", perm.AccessModeRead, "git-upload-pack", "")
			require.NoError(t, extra.Error)
			assert.Equal(t, botName, results.UserName)
		})

		t.Run("web sign-in is refused", func(t *testing.T) {
			for _, password := range []string{"", botToken.Token} {
				req := NewRequestWithValues(t, "POST", "/user/login", map[string]string{
					"user_name": botName,
					"password":  password,
				})
				MakeRequest(t, req, http.StatusOK) // the form is re-rendered with an error instead of redirecting
			}
		})

		t.Run("password cannot be set afterwards", func(t *testing.T) {
			req := NewRequestWithJSON(t, "PATCH", "/api/v1/admin/users/"+botName, api.EditUserOption{
				Password: "some-password-123",
			}).AddTokenAuth(adminToken)
			MakeRequest(t, req, http.StatusUnprocessableEntity)
			assert.False(t, unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: botName}).IsPasswordSet())
		})

		t.Run("other attributes can be edited", func(t *testing.T) {
			req := NewRequestWithJSON(t, "PATCH", "/api/v1/admin/users/"+botName, api.EditUserOption{
				FullName: new("Deploy Bot"),
			}).AddTokenAuth(adminToken)
			MakeRequest(t, req, http.StatusOK)
			assert.Equal(t, "Deploy Bot", unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: botName}).FullName)
		})
	})
}
