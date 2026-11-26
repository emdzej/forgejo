// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package structs

// CreateOrgBotOption options for creating a bot owned by an organization
type CreateOrgBotOption struct {
	// required: true
	Username    string `json:"username" binding:"Required;Username;MaxSize(40)"`
	FullName    string `json:"full_name" binding:"MaxSize(100)"`
	Description string `json:"description" binding:"MaxSize(255)"`
	// IDs of teams of the organization to add the bot to
	TeamIDs []int64 `json:"team_ids"`
}

// EditOrgBotOption options for editing a bot owned by an organization
type EditOrgBotOption struct {
	FullName    *string `json:"full_name" binding:"MaxSize(100)"`
	Description *string `json:"description" binding:"MaxSize(255)"`
}
