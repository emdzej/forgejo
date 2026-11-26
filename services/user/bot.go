// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package user

import (
	"context"
	"fmt"

	"forgejo.org/models/db"
	"forgejo.org/models/organization"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/optional"
	"forgejo.org/modules/util"
	org_service "forgejo.org/services/org"
)

// ErrOrgBotNotManageable is returned when an organization owner tries to manage an
// organization bot that a site administrator granted extra privileges to.
var ErrOrgBotNotManageable = util.NewPermissionDeniedErrorf("this bot can only be managed by site administrators")

// CreateOrgBotOptions are the attributes an organization owner can set on a new bot.
type CreateOrgBotOptions struct {
	Name        string
	FullName    string
	Description string
	// Teams of the organization the bot is added to.
	TeamIDs []int64
}

// CreateOrgBot creates a bot user owned by the organization.
//
// Organization bots are restricted accounts that cannot create repositories or
// organizations: they only reach what the organization grants them through teams or
// collaborations, so an organization owner cannot use them to reach beyond the
// organization.
func CreateOrgBot(ctx context.Context, org *organization.Organization, doer *user_model.User, opts CreateOrgBotOptions) (*user_model.User, error) {
	teams := make([]*organization.Team, 0, len(opts.TeamIDs))
	for _, id := range opts.TeamIDs {
		team, err := organization.GetTeamByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if team.OrgID != org.ID {
			return nil, organization.ErrTeamNotExist{OrgID: org.ID, TeamID: id}
		}
		teams = append(teams, team)
	}

	bot := &user_model.User{
		Name:        opts.Name,
		FullName:    opts.FullName,
		Description: opts.Description,
		Type:        user_model.UserTypeBot,
	}
	noRepos := 0
	overwrite := &user_model.CreateUserOverwriteOptions{
		IsActive:                optional.Some(true),
		IsRestricted:            optional.Some(true),
		KeepEmailPrivate:        optional.Some(true),
		AllowCreateOrganization: optional.Some(false),
		MaxRepoCreation:         &noRepos,
	}

	err := db.WithTx(ctx, func(ctx context.Context) error {
		if err := user_model.CreateOrgBotUser(ctx, bot, overwrite); err != nil {
			return err
		}
		if err := organization.InsertOrgBot(ctx, org.ID, bot.ID); err != nil {
			return err
		}
		for _, team := range teams {
			if err := org_service.AddTeamMemberByCooptation(ctx, team, bot, doer); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bot, nil
}

// CheckOrgBotManageable verifies that doer may manage the organization bot (edit it,
// delete it, manage its tokens and keys). Organization owners can manage the bots of their
// organization unless a site administrator made the bot an administrator.
func CheckOrgBotManageable(doer, bot *user_model.User) error {
	if bot.IsAdmin && !doer.IsAdmin {
		return ErrOrgBotNotManageable
	}
	return nil
}

// DeleteOrgBot deletes a bot owned by the organization. Content created by the bot is
// kept and attributed to the ghost user.
func DeleteOrgBot(ctx context.Context, org *organization.Organization, bot *user_model.User) error {
	// Leaving the organization first fails with ErrLastOrgOwner if the bot is the last
	// owner, instead of cascading into deleting the organization.
	if err := org_service.RemoveOrgUser(ctx, org.ID, bot.ID); err != nil {
		return err
	}
	return DeleteUser(ctx, bot, false)
}

// DeleteOrgBots purges all bots of an organization that is being deleted.
func DeleteOrgBots(ctx context.Context, org *organization.Organization) error {
	bots, err := organization.FindOrgBots(ctx, org.ID)
	if err != nil {
		return err
	}
	for _, bot := range bots {
		// The organization is going away, so the last-owner rule does not apply: drop the
		// memberships directly so that purging the bot does not recurse into the organization.
		if err := db.DeleteBeans(ctx,
			&organization.TeamUser{UID: bot.ID, OrgID: org.ID},
			&organization.OrgUser{UID: bot.ID, OrgID: org.ID},
		); err != nil {
			return err
		}
		if err := DeleteUser(ctx, bot, true); err != nil {
			return fmt.Errorf("delete bot %s of organization %s: %w", bot.Name, org.Name, err)
		}
	}
	return nil
}
