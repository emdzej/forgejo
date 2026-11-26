// Copyright 2024 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"testing"

	auth_model "forgejo.org/models/auth"
	"forgejo.org/models/db"
	"forgejo.org/models/unittest"
	user_model "forgejo.org/models/user"
	"forgejo.org/tests"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func Test_Cmd_AdminUser(t *testing.T) {
	onApplicationRun(t, func(*testing.T, *url.URL) {
		for i, testCase := range []struct {
			name                     string
			options                  []string
			mustChangePassword       bool
			mustChangePasswordUpdate bool
		}{
			{
				name:                     "default",
				options:                  []string{},
				mustChangePassword:       true,
				mustChangePasswordUpdate: false,
			},
			{
				name:                     "--must-change-password=false",
				options:                  []string{"--must-change-password=false"},
				mustChangePassword:       false,
				mustChangePasswordUpdate: false,
			},
			{
				name:                     "--must-change-password=true",
				options:                  []string{"--must-change-password=true"},
				mustChangePassword:       true,
				mustChangePasswordUpdate: true,
			},
			{
				name:                     "--must-change-password",
				options:                  []string{"--must-change-password"},
				mustChangePassword:       true,
				mustChangePasswordUpdate: true,
			},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				defer tests.PrintCurrentTest(t)()
				name := fmt.Sprintf("testuser%d", i)

				options := []string{"user", "create", "--username", name, "--password", "password", "--email", name + "@example.com"}
				options = append(options, testCase.options...)
				output, err := runMainApp("admin", options...)
				require.NoError(t, err)
				assert.Contains(t, output, "has been successfully created")
				user := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: name})
				assert.Equal(t, testCase.mustChangePassword, user.MustChangePassword)

				options = []string{"user", "change-password", "--username", name, "--password", "password"}
				options = append(options, testCase.options...)
				output, err = runMainApp("admin", options...)
				require.NoError(t, err)
				assert.Contains(t, output, "has been successfully updated")
				user = unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: name})
				assert.Equal(t, testCase.mustChangePasswordUpdate, user.MustChangePassword)

				_, err = runMainApp("admin", "user", "delete", "--username", name)
				require.NoError(t, err)
				unittest.AssertNotExistsBean(t, &user_model.User{Name: name})
			})
		}
	})
}

func Test_Cmd_AdminFirstUser(t *testing.T) {
	onApplicationRun(t, func(*testing.T, *url.URL) {
		for _, testCase := range []struct {
			name               string
			options            []string
			mustChangePassword bool
			isAdmin            bool
		}{
			{
				name:               "default",
				options:            []string{},
				mustChangePassword: false,
				isAdmin:            false,
			},
			{
				name:               "--must-change-password=false",
				options:            []string{"--must-change-password=false"},
				mustChangePassword: false,
				isAdmin:            false,
			},
			{
				name:               "--must-change-password=true",
				options:            []string{"--must-change-password=true"},
				mustChangePassword: true,
				isAdmin:            false,
			},
			{
				name:               "--must-change-password",
				options:            []string{"--must-change-password"},
				mustChangePassword: true,
				isAdmin:            false,
			},
			{
				name:               "--admin default",
				options:            []string{"--admin"},
				mustChangePassword: false,
				isAdmin:            true,
			},
			{
				name:               "--admin --must-change-password=false",
				options:            []string{"--admin", "--must-change-password=false"},
				mustChangePassword: false,
				isAdmin:            true,
			},
			{
				name:               "--admin --must-change-password=true",
				options:            []string{"--admin", "--must-change-password=true"},
				mustChangePassword: true,
				isAdmin:            true,
			},
			{
				name:               "--admin --must-change-password",
				options:            []string{"--admin", "--must-change-password"},
				mustChangePassword: true,
				isAdmin:            true,
			},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				db.TruncateBeansCascade(db.DefaultContext, user_model.User{})
				db.GetEngine(db.DefaultContext).Exec("DELETE FROM `email_address`")
				assert.Equal(t, int64(0), user_model.CountUsers(db.DefaultContext, nil))
				name := "testuser"

				options := []string{"user", "create", "--username", name, "--password", "password", "--email", name + "@example.com"}
				options = append(options, testCase.options...)
				output, err := runMainApp("admin", options...)
				require.NoError(t, err)
				assert.Contains(t, output, "has been successfully created")
				user := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: name})
				assert.Equal(t, testCase.mustChangePassword, user.MustChangePassword)
				assert.Equal(t, testCase.isAdmin, user.IsAdmin)
			})
		}
	})
}

func Test_Cmd_AdminUserResetMFA(t *testing.T) {
	onApplicationRun(t, func(*testing.T, *url.URL) {
		name := "testuser"

		options := []string{"user", "create", "--username", name, "--password", "password", "--email", name + "@example.com"}
		output, err := runMainApp("admin", options...)
		require.NoError(t, err)
		assert.Contains(t, output, "has been successfully created")
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: name})

		twoFactor := &auth_model.TwoFactor{
			UID: user.ID,
		}
		token := twoFactor.GenerateScratchToken()
		require.NoError(t, auth_model.NewTwoFactor(t.Context(), twoFactor, token))
		twoFactor, err = auth_model.GetTwoFactorByUID(t.Context(), user.ID)
		require.NoError(t, err)
		require.NotNil(t, twoFactor)

		authn, err := auth_model.CreateCredential(t.Context(), user.ID, "test", &webauthn.Credential{})
		require.NoError(t, err)

		options = []string{"user", "reset-mfa", "--username", name}
		output, err = runMainApp("admin", options...)
		require.NoError(t, err)
		assert.Contains(t, output, "two-factor authentication settings have been removed")

		_, err = auth_model.GetTwoFactorByUID(t.Context(), user.ID)
		require.ErrorContains(t, err, "user not enrolled in 2FA")

		_, err = auth_model.GetWebAuthnCredentialByID(t.Context(), authn.ID)
		require.ErrorContains(t, err, "WebAuthn credential does not exist")

		_, err = runMainApp("admin", "user", "delete", "--username", name)
		require.NoError(t, err)
	})
}

func Test_Cmd_AdminUserStdin(t *testing.T) {
	onApplicationRun(t, func(t *testing.T, _ *url.URL) {
		defer tests.PrintCurrentTest(t)()
		require.NoError(t, unittest.PrepareTestDatabase())

		rIn, wIn, err := os.Pipe()
		require.NoError(t, err)

		cases := []struct {
			stdin   string
			changes bool
		}{
			{
				stdin:   "password3\npassword3\n",
				changes: true,
			},
			{
				stdin:   "password3\nhunter2\n",
				changes: false,
			},
		}

		for _, c := range cases {
			before := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 44})
			assert.False(t, before.MustChangePassword)

			var group errgroup.Group
			group.Go(func() error {
				_, err := tests.RunMainAppWithStdin(rIn, "admin", []string{"user", "change-password", "-u", "user44"}...)
				return err
			})

			_, err = wIn.Write([]byte(c.stdin))
			require.NoError(t, err)

			cmdErr := group.Wait()

			after := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 44})
			assert.False(t, after.MustChangePassword)

			if c.changes {
				require.NoError(t, cmdErr)
				assert.NotEqual(t, before.Passwd, after.Passwd)
			} else {
				require.Error(t, cmdErr)
				assert.Equal(t, before.Passwd, after.Passwd)
			}
		}
	})
}

func Test_Cmd_AdminUserCreateBot(t *testing.T) {
	onApplicationRun(t, func(t *testing.T, _ *url.URL) {
		t.Run("bot without password or email", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()
			output, err := runMainApp("admin", "user", "create", "--username", "cli-bot", "--user-type", "bot",
				"--access-token", "--access-token-name", "ci", "--access-token-scopes", "read:repository")
			require.NoError(t, err)
			assert.Contains(t, output, "has been successfully created")
			assert.Contains(t, output, "Access token was successfully created")

			bot := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "cli-bot"})
			assert.True(t, bot.IsBot())
			assert.False(t, bot.IsPasswordSet())
			assert.False(t, bot.MustChangePassword)
			assert.NotEmpty(t, bot.Email)
			unittest.AssertExistsAndLoadBean(t, &auth_model.AccessToken{UID: bot.ID, Name: "ci"})

			// a password cannot be added later on either
			_, err = runMainApp("admin", "user", "change-password", "--username", "cli-bot", "--password", "password-123456")
			assertCmdStderrContains(t, err, "bot users cannot have a password")
			assert.False(t, unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "cli-bot"}).IsPasswordSet())
		})

		for _, flags := range [][]string{
			{"--password", "password-123456"},
			{"--random-password"},
			{"--must-change-password"},
		} {
			t.Run("bot refuses "+flags[0], func(t *testing.T) {
				defer tests.PrintCurrentTest(t)()
				args := append([]string{"user", "create", "--username", "cli-bad-bot", "--user-type", "bot"}, flags...)
				_, err := runMainApp("admin", args...)
				assertCmdStderrContains(t, err, "cannot be used with --user-type bot")
				unittest.AssertNotExistsBean(t, &user_model.User{Name: "cli-bad-bot"})
			})
		}

		t.Run("unknown user type", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()
			_, err := runMainApp("admin", "user", "create", "--username", "cli-robot", "--user-type", "robot",
				"--email", "robot@example.com", "--password", "password-123456")
			assertCmdStderrContains(t, err, "invalid --user-type")
		})
	})
}

// assertCmdStderrContains checks that a runMainApp subprocess failed with the given message on stderr.
func assertCmdStderrContains(t *testing.T, err error, msg string) {
	t.Helper()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Contains(t, string(exitErr.Stderr), msg)
}
