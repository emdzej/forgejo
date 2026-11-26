// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package user

import (
	"fmt"
	"strings"

	"forgejo.org/models/auth"
	"forgejo.org/modules/setting"
	"forgejo.org/modules/util"
)

// ErrBotUserInvalid is returned when a bot user is created with attributes a bot cannot have.
type ErrBotUserInvalid struct {
	Name   string
	Reason string
}

func IsErrBotUserInvalid(err error) bool {
	_, ok := err.(ErrBotUserInvalid)
	return ok
}

func (err ErrBotUserInvalid) Error() string {
	return fmt.Sprintf("invalid bot user [name: %s]: %s", err.Name, err.Reason)
}

func (err ErrBotUserInvalid) Unwrap() error {
	return util.ErrInvalidArgument
}

// BotPlaceholderEmail returns the no-reply address used for bots created without an email.
func BotPlaceholderEmail(name string) string {
	return fmt.Sprintf("%s@%s", strings.ToLower(name), setting.Service.NoReplyAddress)
}

// prepareBotUser enforces the invariants of bot users before they are inserted.
//
// Bots are local accounts that authenticate only with access tokens (API, git over HTTP)
// or SSH keys. They never have a password, so they cannot sign in to the web UI; password
// sign-in of non-individual users is additionally refused in services/auth/source/db.
func prepareBotUser(u *User, creator userCreator) error {
	switch creator {
	case userCreatorAdmin:
	case userCreatorOrgOwner:
		// Organization owners must not be able to claim email addresses, nor to create
		// accounts with privileges beyond the organization.
		if u.Email != "" {
			return ErrBotUserInvalid{Name: u.Name, Reason: "organization bots cannot have a custom email"}
		}
		if u.IsAdmin {
			return ErrBotUserInvalid{Name: u.Name, Reason: "organization bots cannot be administrators"}
		}
	default:
		return ErrBotUserInvalid{Name: u.Name, Reason: "bot users can only be created by administrators or organization owners"}
	}
	if u.Passwd != "" {
		return ErrBotUserInvalid{Name: u.Name, Reason: "bot users cannot have a password"}
	}
	if u.LoginSource != 0 || (u.LoginType != auth.NoType && u.LoginType != auth.Plain) {
		return ErrBotUserInvalid{Name: u.Name, Reason: "bot users cannot use an external authentication source"}
	}

	u.LoginType = auth.Plain
	u.LoginName = ""
	u.MustChangePassword = false
	if u.Email == "" {
		u.Email = BotPlaceholderEmail(u.Name)
	}
	return nil
}
