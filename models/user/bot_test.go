// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package user_test

import (
	"testing"

	"forgejo.org/models/auth"
	"forgejo.org/models/db"
	"forgejo.org/models/unittest"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/setting"
	"forgejo.org/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminCreateBotUser(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Service.NoReplyAddress, "noreply.example.org")()

	t.Run("without email gets a no-reply address", func(t *testing.T) {
		bot := &user_model.User{Name: "Deploy-Bot", Type: user_model.UserTypeBot, MustChangePassword: true}
		require.NoError(t, user_model.AdminCreateUser(db.DefaultContext, bot))

		loaded := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: bot.ID})
		assert.True(t, loaded.IsBot())
		assert.Equal(t, "deploy-bot@noreply.example.org", loaded.Email)
		assert.False(t, loaded.IsPasswordSet())
		assert.False(t, loaded.MustChangePassword)
		assert.Equal(t, auth.Plain, loaded.LoginType)
		unittest.AssertExistsAndLoadBean(t, &user_model.EmailAddress{UID: bot.ID, IsPrimary: true})
	})

	t.Run("with email", func(t *testing.T) {
		bot := &user_model.User{Name: "ci-bot", Email: "ci-bot@example.com", Type: user_model.UserTypeBot}
		require.NoError(t, user_model.AdminCreateUser(db.DefaultContext, bot))
		assert.Equal(t, "ci-bot@example.com", unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: bot.ID}).Email)
	})

	t.Run("with password is refused", func(t *testing.T) {
		bot := &user_model.User{Name: "pw-bot", Passwd: "secret-password", Type: user_model.UserTypeBot}
		err := user_model.AdminCreateUser(db.DefaultContext, bot)
		require.Error(t, err)
		assert.True(t, user_model.IsErrBotUserInvalid(err))
		unittest.AssertNotExistsBean(t, &user_model.User{Name: "pw-bot"})
	})

	t.Run("with external login source is refused", func(t *testing.T) {
		bot := &user_model.User{Name: "ldap-bot", LoginType: auth.LDAP, LoginSource: 1, Type: user_model.UserTypeBot}
		err := user_model.AdminCreateUser(db.DefaultContext, bot)
		require.Error(t, err)
		assert.True(t, user_model.IsErrBotUserInvalid(err))
	})

	t.Run("by non-admin path is refused", func(t *testing.T) {
		bot := &user_model.User{Name: "self-bot", Email: "self-bot@example.com", Type: user_model.UserTypeBot}
		err := user_model.CreateUser(db.DefaultContext, bot)
		require.Error(t, err)
		assert.True(t, user_model.IsErrBotUserInvalid(err))
		unittest.AssertNotExistsBean(t, &user_model.User{Name: "self-bot"})
	})
}
