// Copyright 2015 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import "time"

// CreateUserOption create user options
type CreateUserOption struct {
	SourceID  int64  `json:"source_id"`
	LoginName string `json:"login_name"`
	// required: true
	Username string `json:"username" binding:"Required;Username;MaxSize(40)"`
	FullName string `json:"full_name" binding:"MaxSize(100)"`
	// Required unless `user_type` is `bot`. Bots created without an email get a no-reply address.
	// swagger:strfmt email
	Email string `json:"email" binding:"OmitEmpty;EmailForAdmin;MaxSize(254)"`
	// Type of the account. A `bot` has no password and cannot sign in to the web UI; it
	// authenticates with access tokens or SSH keys only. `password`, `must_change_password`,
	// `source_id` and `login_name` must not be set for bots.
	// enum: individual,bot
	// default: individual
	UserType           string `json:"user_type" binding:"In(,individual,bot)"`
	Password           string `json:"password" binding:"MaxSize(255)"`
	MustChangePassword *bool  `json:"must_change_password"`
	SendNotify         bool   `json:"send_notify"`
	Restricted         *bool  `json:"restricted"`
	Visibility         string `json:"visibility" binding:"In(,public,limited,private)"`

	// For explicitly setting the user creation timestamp. Useful when users are
	// migrated from other systems. When omitted, the user's creation timestamp
	// will be set to "now".
	Created *time.Time `json:"created_at"`
}

// EditUserOption edit user options
type EditUserOption struct {
	SourceID  *int64  `json:"source_id"`
	LoginName *string `json:"login_name"`
	// swagger:strfmt email
	Email                   *string `json:"email" binding:"MaxSize(254)"`
	FullName                *string `json:"full_name" binding:"MaxSize(100)"`
	Password                string  `json:"password" binding:"MaxSize(255)"`
	MustChangePassword      *bool   `json:"must_change_password"`
	Website                 *string `json:"website" binding:"OmitEmpty;ValidSiteUrl;MaxSize(255)"`
	Location                *string `json:"location" binding:"MaxSize(50)"`
	Pronouns                *string `json:"pronouns" binding:"MaxSize(50)"`
	Description             *string `json:"description" binding:"MaxSize(255)"`
	Active                  *bool   `json:"active"`
	Admin                   *bool   `json:"admin"`
	AllowGitHook            *bool   `json:"allow_git_hook"`
	AllowImportLocal        *bool   `json:"allow_import_local"`
	MaxRepoCreation         *int    `json:"max_repo_creation"`
	ProhibitLogin           *bool   `json:"prohibit_login"`
	AllowCreateOrganization *bool   `json:"allow_create_organization"`
	Restricted              *bool   `json:"restricted"`
	Visibility              string  `json:"visibility" binding:"In(,public,limited,private)"`
	HideEmail               *bool   `json:"hide_email"`
}
