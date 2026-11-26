// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package integration

import (
	"fmt"
	"net/http"
	"testing"

	auth_model "forgejo.org/models/auth"
	"forgejo.org/models/organization"
	"forgejo.org/models/unittest"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/optional"
	"forgejo.org/modules/setting"
	api "forgejo.org/modules/structs"
	"forgejo.org/modules/test"
	"forgejo.org/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures: user2 owns org3; user4 is a member of org3's team1 (id 2), which has access to
// the private org3/repo3. Team 3 belongs to org6.

func createOrgBot(t *testing.T, token, org string, option api.CreateOrgBotOption) *user_model.User {
	t.Helper()
	req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/"+org+"/bots", option).AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusCreated)
	var apiBot api.User
	DecodeJSON(t, resp, &apiBot)
	assert.True(t, apiBot.IsBot)
	return unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: apiBot.ID})
}

func createOrgBotToken(t *testing.T, token, org, bot string, scopes ...string) string {
	t.Helper()
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/orgs/%s/bots/%s/tokens", org, bot), api.CreateAccessTokenOption{
		Name:   "ci",
		Scopes: scopes,
	}).AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusCreated)
	var apiToken api.AccessToken
	DecodeJSON(t, resp, &apiToken)
	return apiToken.Token
}

func TestAPIOrgBot(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	ownerToken := getUserToken(t, "user2", auth_model.AccessTokenScopeWriteOrganization)
	memberToken := getUserToken(t, "user4", auth_model.AccessTokenScopeWriteOrganization)
	adminToken := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteAdmin, auth_model.AccessTokenScopeWriteOrganization, auth_model.AccessTokenScopeWriteRepository)

	bot := createOrgBot(t, ownerToken, "org3", api.CreateOrgBotOption{
		Username: "org3-ci",
		FullName: "CI",
		TeamIDs:  []int64{2},
	})

	t.Run("created with the organization bot defaults", func(t *testing.T) {
		assert.True(t, bot.IsBot())
		assert.True(t, bot.IsRestricted)
		assert.True(t, bot.IsActive)
		assert.False(t, bot.IsPasswordSet())
		assert.False(t, bot.AllowCreateOrganization)
		assert.Equal(t, 0, bot.MaxRepoCreation)
		assert.Equal(t, "org3-ci@"+setting.Service.NoReplyAddress, bot.Email)
		unittest.AssertExistsAndLoadBean(t, &organization.OrgBot{OrgID: 3, BotID: bot.ID})
		unittest.AssertExistsAndLoadBean(t, &organization.TeamUser{TeamID: 2, UID: bot.ID})
	})

	t.Run("creation is validated", func(t *testing.T) {
		for _, option := range []api.CreateOrgBotOption{
			{Username: "org3-ci"},                           // taken
			{Username: "foreign-team", TeamIDs: []int64{3}}, // team of org6
			{Username: "bad name"},
		} {
			req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/org3/bots", option).AddTokenAuth(ownerToken)
			MakeRequest(t, req, http.StatusUnprocessableEntity)
		}
		unittest.AssertNotExistsBean(t, &user_model.User{Name: "foreign-team"})
	})

	t.Run("only owners manage bots", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots").AddTokenAuth(memberToken), http.StatusForbidden)
		req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/org3/bots", api.CreateOrgBotOption{Username: "member-bot"}).AddTokenAuth(memberToken)
		MakeRequest(t, req, http.StatusForbidden)
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/org3-ci/tokens").AddTokenAuth(memberToken), http.StatusForbidden)
		unittest.AssertNotExistsBean(t, &user_model.User{Name: "member-bot"})
	})

	t.Run("list and get", func(t *testing.T) {
		resp := MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots").AddTokenAuth(ownerToken), http.StatusOK)
		var bots []*api.User
		DecodeJSON(t, resp, &bots)
		require.Len(t, bots, 1)
		assert.Equal(t, "org3-ci", bots[0].UserName)

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/org3-ci").AddTokenAuth(ownerToken), http.StatusOK)
		// users that are not bots of this organization are not found
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/user4").AddTokenAuth(ownerToken), http.StatusNotFound)
	})

	t.Run("edit", func(t *testing.T) {
		req := NewRequestWithJSON(t, "PATCH", "/api/v1/orgs/org3/bots/org3-ci", api.EditOrgBotOption{
			FullName:    new("Continuous Integration"),
			Description: new("Runs the pipelines"),
		}).AddTokenAuth(ownerToken)
		MakeRequest(t, req, http.StatusOK)
		edited := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: bot.ID})
		assert.Equal(t, "Continuous Integration", edited.FullName)
		assert.Equal(t, "Runs the pipelines", edited.Description)
	})

	t.Run("token reaches only what the organization grants", func(t *testing.T) {
		botToken := createOrgBotToken(t, ownerToken, "org3", "org3-ci", "read:repository", "read:user")

		resp := MakeRequest(t, NewRequest(t, "GET", "/api/v1/user").AddTokenAuth(botToken), http.StatusOK)
		var self api.User
		DecodeJSON(t, resp, &self)
		assert.Equal(t, "org3-ci", self.UserName)

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/org3/repo3").AddTokenAuth(botToken), http.StatusOK)
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/repo2").AddTokenAuth(botToken), http.StatusNotFound)

		resp = MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/org3-ci/tokens").AddTokenAuth(ownerToken), http.StatusOK)
		var tokens []*api.AccessToken
		DecodeJSON(t, resp, &tokens)
		require.Len(t, tokens, 1)

		MakeRequest(t, NewRequestf(t, "DELETE", "/api/v1/orgs/org3/bots/org3-ci/tokens/%d", tokens[0].ID).AddTokenAuth(ownerToken), http.StatusNoContent)
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/user").AddTokenAuth(botToken), http.StatusUnauthorized)
	})

	t.Run("ssh keys", func(t *testing.T) {
		req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/org3/bots/org3-ci/keys", api.CreateKeyOption{
			Title: "deploy",
			Key:   botTestSSHKey,
		}).AddTokenAuth(ownerToken)
		resp := MakeRequest(t, req, http.StatusCreated)
		var key api.PublicKey
		DecodeJSON(t, resp, &key)

		resp = MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/org3-ci/keys").AddTokenAuth(ownerToken), http.StatusOK)
		var keys []*api.PublicKey
		DecodeJSON(t, resp, &keys)
		require.Len(t, keys, 1)
		assert.Equal(t, key.ID, keys[0].ID)

		MakeRequest(t, NewRequestf(t, "DELETE", "/api/v1/orgs/org3/bots/org3-ci/keys/%d", key.ID).AddTokenAuth(ownerToken), http.StatusNoContent)
	})

	t.Run("cannot join other organizations", func(t *testing.T) {
		// even a site administrator cannot add it to a team of org6
		MakeRequest(t, NewRequest(t, "PUT", "/api/v1/teams/3/members/org3-ci").AddTokenAuth(adminToken), http.StatusForbidden)
		unittest.AssertNotExistsBean(t, &organization.TeamUser{TeamID: 3, UID: bot.ID})

		req := NewRequestWithJSON(t, "PUT", "/api/v1/repos/user2/repo1/collaborators/org3-ci", api.AddCollaboratorOption{}).AddTokenAuth(adminToken)
		MakeRequest(t, req, http.StatusForbidden)

		// but it can be added to the teams and repositories of its organization
		MakeRequest(t, NewRequest(t, "PUT", "/api/v1/teams/7/members/org3-ci").AddTokenAuth(ownerToken), http.StatusNoContent)
		req = NewRequestWithJSON(t, "PUT", "/api/v1/repos/org3/repo3/collaborators/org3-ci", api.AddCollaboratorOption{}).AddTokenAuth(adminToken)
		MakeRequest(t, req, http.StatusNoContent)
	})

	t.Run("joins teams directly when membership requires invitations", func(t *testing.T) {
		defer test.MockVariableValue(&setting.Service.AddMembersByInvitations, true)()
		other := createOrgBot(t, ownerToken, "org3", api.CreateOrgBotOption{Username: "org3-invited"})
		MakeRequest(t, NewRequest(t, "PUT", "/api/v1/teams/2/members/org3-invited").AddTokenAuth(ownerToken), http.StatusNoContent)
		unittest.AssertExistsAndLoadBean(t, &organization.TeamUser{TeamID: 2, UID: other.ID})
		unittest.AssertNotExistsBean(t, &organization.TeamInvite{InvitedID: optional.Some(other.ID)})
		MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/org3/bots/org3-invited").AddTokenAuth(ownerToken), http.StatusNoContent)
	})

	t.Run("bots made administrators are managed by site administrators only", func(t *testing.T) {
		req := NewRequestWithJSON(t, "PATCH", "/api/v1/admin/users/org3-ci", api.EditUserOption{Admin: new(true)}).AddTokenAuth(adminToken)
		MakeRequest(t, req, http.StatusOK)
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/orgs/org3/bots/org3-ci/tokens").AddTokenAuth(ownerToken), http.StatusForbidden)
		MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/org3/bots/org3-ci").AddTokenAuth(ownerToken), http.StatusForbidden)

		req = NewRequestWithJSON(t, "PATCH", "/api/v1/admin/users/org3-ci", api.EditUserOption{Admin: new(false)}).AddTokenAuth(adminToken)
		MakeRequest(t, req, http.StatusOK)
	})

	t.Run("delete", func(t *testing.T) {
		MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/org3/bots/org3-ci").AddTokenAuth(ownerToken), http.StatusNoContent)
		unittest.AssertNotExistsBean(t, &user_model.User{ID: bot.ID})
		unittest.AssertNotExistsBean(t, &organization.OrgBot{BotID: bot.ID})
		unittest.AssertNotExistsBean(t, &organization.TeamUser{UID: bot.ID})
	})
}

func TestAPIOrgBotBlocksOrgDeletion(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	ownerToken := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteOrganization)
	req := NewRequestWithJSON(t, "POST", "/api/v1/orgs", api.CreateOrgOption{UserName: "bot-org"}).AddTokenAuth(ownerToken)
	MakeRequest(t, req, http.StatusCreated)
	bot := createOrgBot(t, ownerToken, "bot-org", api.CreateOrgBotOption{Username: "bot-org-ci"})

	MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/bot-org").AddTokenAuth(ownerToken), http.StatusUnprocessableEntity)
	unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: bot.ID})

	MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/bot-org/bots/bot-org-ci").AddTokenAuth(ownerToken), http.StatusNoContent)
	MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/orgs/bot-org").AddTokenAuth(ownerToken), http.StatusNoContent)
}

func TestAPIOrgBotPurgedWithOrg(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminToken := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteAdmin, auth_model.AccessTokenScopeWriteOrganization)

	// a user who is the only owner of an organization owning a bot
	req := NewRequestWithJSON(t, "POST", "/api/v1/admin/users", api.CreateUserOption{
		Username: "purged-owner", Email: "purged-owner@example.com", Password: "some-password-123",
		MustChangePassword: new(false),
	}).AddTokenAuth(adminToken)
	MakeRequest(t, req, http.StatusCreated)
	req = NewRequestWithJSON(t, "POST", "/api/v1/admin/users/purged-owner/orgs", api.CreateOrgOption{UserName: "purged-org"}).AddTokenAuth(adminToken)
	MakeRequest(t, req, http.StatusCreated)
	bot := createOrgBot(t, adminToken, "purged-org", api.CreateOrgBotOption{Username: "purged-org-ci"})

	MakeRequest(t, NewRequest(t, "DELETE", "/api/v1/admin/users/purged-owner?purge=true").AddTokenAuth(adminToken), http.StatusNoContent)
	unittest.AssertNotExistsBean(t, &user_model.User{Name: "purged-owner"})
	unittest.AssertNotExistsBean(t, &user_model.User{Name: "purged-org"})
	unittest.AssertNotExistsBean(t, &user_model.User{ID: bot.ID})
	unittest.AssertNotExistsBean(t, &organization.OrgBot{BotID: bot.ID})
}
