// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package integration

import (
	"fmt"
	"net/http"
	"testing"

	asymkey_model "forgejo.org/models/asymkey"
	auth_model "forgejo.org/models/auth"
	"forgejo.org/models/organization"
	"forgejo.org/models/unittest"
	user_model "forgejo.org/models/user"
	"forgejo.org/tests"

	"github.com/stretchr/testify/assert"
)

func TestAdminWebBot(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	session := loginUser(t, "user1")

	// the form offers the account type
	resp := session.MakeRequest(t, NewRequest(t, "GET", "/admin/users/new"), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, "#user_type", true)

	// a bot needs neither password nor email; authentication fields are ignored
	req := NewRequestWithValues(t, "POST", "/admin/users/new", map[string]string{
		"user_type":  "bot",
		"login_type": "0-0",
		"user_name":  "web-bot",
		"visibility": "0",
		"password":   "ignored-password-123",
	})
	resp = session.MakeRequest(t, req, http.StatusSeeOther)
	bot := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "web-bot"})
	assert.True(t, bot.IsBot())
	assert.False(t, bot.IsPasswordSet())
	botLink := fmt.Sprintf("/admin/users/%d", bot.ID)
	assert.Equal(t, botLink+"/edit", resp.Header().Get("Location"))

	// individuals still need an email
	req = NewRequestWithValues(t, "POST", "/admin/users/new", map[string]string{
		"user_type":  "individual",
		"login_type": "0-0",
		"user_name":  "no-email-user",
		"visibility": "0",
		"password":   "some-password-123",
	})
	session.MakeRequest(t, req, http.StatusOK)
	unittest.AssertNotExistsBean(t, &user_model.User{Name: "no-email-user"})

	t.Run("edit page manages credentials", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", botLink+"/edit"), http.StatusOK)
		page := NewHTMLParser(t, resp.Body)
		page.AssertElement(t, "#bot-account-info", true)
		page.AssertElement(t, "#bot-tokens", true)
		page.AssertElement(t, "#bot-keys", true)
		assert.True(t, page.Find("#password").Closest(".field").HasClass("tw-hidden"))

		// regular users have no such section
		resp = session.MakeRequest(t, NewRequest(t, "GET", "/admin/users/2/edit"), http.StatusOK)
		NewHTMLParser(t, resp.Body).AssertElement(t, "#bot-tokens", false)
		session.MakeRequest(t, NewRequest(t, "GET", "/admin/users/2/tokens/new"), http.StatusNotFound)
	})

	t.Run("tokens", func(t *testing.T) {
		session.MakeRequest(t, NewRequest(t, "GET", botLink+"/tokens/new"), http.StatusOK)
		req := NewRequestWithValues(t, "POST", botLink+"/tokens/new", map[string]string{
			"name":     "ci",
			"resource": "all",
			"scope":    "read:repository",
		})
		resp := session.MakeRequest(t, req, http.StatusSeeOther)
		assert.Equal(t, botLink+"/edit", resp.Header().Get("Location"))
		token := unittest.AssertExistsAndLoadBean(t, &auth_model.AccessToken{UID: bot.ID, Name: "ci"})
		unittest.AssertNotExistsBean(t, &auth_model.AccessToken{UID: 1, Name: "ci"})

		req = NewRequestWithValues(t, "POST", botLink+"/tokens/delete", map[string]string{"id": fmt.Sprint(token.ID)})
		session.MakeRequest(t, req, http.StatusOK)
		unittest.AssertNotExistsBean(t, &auth_model.AccessToken{ID: token.ID})
	})

	t.Run("ssh keys", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", botLink+"/keys", map[string]string{
			"title":   "deploy",
			"content": botTestSSHKey,
		})
		session.MakeRequest(t, req, http.StatusSeeOther)
		key := unittest.AssertExistsAndLoadBean(t, &asymkey_model.PublicKey{OwnerID: bot.ID, Name: "deploy"})

		req = NewRequestWithValues(t, "POST", botLink+"/keys/delete", map[string]string{"id": fmt.Sprint(key.ID)})
		session.MakeRequest(t, req, http.StatusOK)
		unittest.AssertNotExistsBean(t, &asymkey_model.PublicKey{ID: key.ID})
	})
}

func TestOrgWebBot(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	session := loginUser(t, "user2") // owner of org3
	botsLink := "/org/org3/settings/bots"

	resp := session.MakeRequest(t, NewRequest(t, "GET", botsLink), http.StatusOK)
	NewHTMLParser(t, resp.Body).AssertElement(t, "#new-org-bot", true)

	// members who are not owners have no access
	loginUser(t, "user4").MakeRequest(t, NewRequest(t, "GET", botsLink), http.StatusNotFound)

	req := NewRequestWithValues(t, "POST", botsLink, map[string]string{
		"user_name": "org3-web-bot",
		"full_name": "Web Bot",
		"team_ids":  "2",
	})
	resp = session.MakeRequest(t, req, http.StatusSeeOther)
	botLink := botsLink + "/org3-web-bot"
	assert.Equal(t, botLink, resp.Header().Get("Location"))
	bot := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "org3-web-bot"})
	assert.True(t, bot.IsBot())
	assert.True(t, bot.IsRestricted)
	unittest.AssertExistsAndLoadBean(t, &organization.OrgBot{OrgID: 3, BotID: bot.ID})
	unittest.AssertExistsAndLoadBean(t, &organization.TeamUser{TeamID: 2, UID: bot.ID})

	t.Run("list and details", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "GET", botsLink), http.StatusOK)
		assert.Contains(t, resp.Body.String(), botLink)

		resp = session.MakeRequest(t, NewRequest(t, "GET", botLink), http.StatusOK)
		page := NewHTMLParser(t, resp.Body)
		page.AssertElement(t, "#edit-org-bot", true)
		page.AssertElement(t, "#bot-tokens", true)
		page.AssertElement(t, "#bot-keys", true)

		// users that are not bots of org3 are not found
		session.MakeRequest(t, NewRequest(t, "GET", botsLink+"/user4"), http.StatusNotFound)
	})

	t.Run("edit", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", botLink, map[string]string{
			"full_name":   "Renamed",
			"description": "Deploys things",
		})
		session.MakeRequest(t, req, http.StatusSeeOther)
		edited := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: bot.ID})
		assert.Equal(t, "Renamed", edited.FullName)
		assert.Equal(t, "Deploys things", edited.Description)
	})

	t.Run("repository-specific token", func(t *testing.T) {
		// the repository picker lists what the bot reaches through its teams
		resp := session.MakeRequest(t, NewRequest(t, "GET", botLink+"/tokens/new"), http.StatusOK)
		assert.Contains(t, resp.Body.String(), "org3/repo3")

		req := NewRequestWithValues(t, "POST", botLink+"/tokens/new", map[string]string{
			"name":          "deploy",
			"resource":      "repo-specific",
			"selected_repo": "org3/repo3",
			"scope":         "write:repository",
		})
		resp = session.MakeRequest(t, req, http.StatusSeeOther)
		assert.Equal(t, botLink, resp.Header().Get("Location"))
		token := unittest.AssertExistsAndLoadBean(t, &auth_model.AccessToken{UID: bot.ID, Name: "deploy"})
		assert.False(t, token.ResourceAllRepos)
		unittest.AssertExistsAndLoadBean(t, &auth_model.AccessTokenResourceRepo{TokenID: token.ID, RepoID: 3})

		// repositories the bot cannot reach are refused
		req = NewRequestWithValues(t, "POST", botLink+"/tokens/new", map[string]string{
			"name":          "sneaky",
			"resource":      "repo-specific",
			"selected_repo": "user2/repo2",
			"scope":         "read:repository",
		})
		session.MakeRequest(t, req, http.StatusBadRequest)
		unittest.AssertNotExistsBean(t, &auth_model.AccessToken{UID: bot.ID, Name: "sneaky"})
	})

	t.Run("ssh keys", func(t *testing.T) {
		req := NewRequestWithValues(t, "POST", botLink+"/keys", map[string]string{
			"title":   "deploy",
			"content": botTestSSHKey,
		})
		session.MakeRequest(t, req, http.StatusSeeOther)
		unittest.AssertExistsAndLoadBean(t, &asymkey_model.PublicKey{OwnerID: bot.ID, Name: "deploy"})
	})

	t.Run("delete", func(t *testing.T) {
		resp := session.MakeRequest(t, NewRequest(t, "POST", botLink+"/delete"), http.StatusOK)
		var redirect map[string]string
		DecodeJSON(t, resp, &redirect)
		assert.Equal(t, botsLink, redirect["redirect"])
		unittest.AssertNotExistsBean(t, &user_model.User{ID: bot.ID})
		unittest.AssertNotExistsBean(t, &asymkey_model.PublicKey{OwnerID: bot.ID})
		unittest.AssertNotExistsBean(t, &auth_model.AccessToken{UID: bot.ID})
	})
}
