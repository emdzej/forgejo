// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package organization

import (
	"context"
	"fmt"

	"forgejo.org/models/db"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/timeutil"
	"forgejo.org/modules/util"
)

// OrgBot records that a bot user is owned by an organization. Owners of the organization
// manage the bot; the bot may only be a member of that organization's teams and a
// collaborator on that organization's repositories.
type OrgBot struct {
	ID          int64              `xorm:"pk autoincr"`
	OrgID       int64              `xorm:"INDEX NOT NULL REFERENCES(user, id)"`
	BotID       int64              `xorm:"UNIQUE NOT NULL REFERENCES(user, id)"`
	CreatedUnix timeutil.TimeStamp `xorm:"created"`
}

func init() {
	db.RegisterModel(new(OrgBot))
}

// ErrOrgBotForeignOrg is returned when an organization bot is used outside of its organization.
type ErrOrgBotForeignOrg struct {
	BotName string
}

func IsErrOrgBotForeignOrg(err error) bool {
	_, ok := err.(ErrOrgBotForeignOrg)
	return ok
}

func (err ErrOrgBotForeignOrg) Error() string {
	return fmt.Sprintf("bot %s belongs to another organization", err.BotName)
}

func (err ErrOrgBotForeignOrg) Unwrap() error {
	return util.ErrPermissionDenied
}

// ErrOrgOwnsBots is returned when deleting an organization that still owns bots.
type ErrOrgOwnsBots struct {
	OrgID int64
}

func IsErrOrgOwnsBots(err error) bool {
	_, ok := err.(ErrOrgOwnsBots)
	return ok
}

func (err ErrOrgOwnsBots) Error() string {
	return fmt.Sprintf("organization still owns bots [org_id: %d]", err.OrgID)
}

// InsertOrgBot records that the bot belongs to the organization.
func InsertOrgBot(ctx context.Context, orgID, botID int64) error {
	return db.Insert(ctx, &OrgBot{OrgID: orgID, BotID: botID})
}

// GetBotOwnerOrgID returns the ID of the organization owning the bot, or 0 when the user
// is not an organization bot.
func GetBotOwnerOrgID(ctx context.Context, userID int64) (int64, error) {
	ob := &OrgBot{}
	has, err := db.GetEngine(ctx).Where("bot_id = ?", userID).Get(ob)
	if err != nil || !has {
		return 0, err
	}
	return ob.OrgID, nil
}

// CheckBotAllowedInOrg returns ErrOrgBotForeignOrg if the user is a bot owned by an
// organization other than orgID. It is a no-op for all other users.
func CheckBotAllowedInOrg(ctx context.Context, u *user_model.User, orgID int64) error {
	if !u.IsBot() {
		return nil
	}
	ownerID, err := GetBotOwnerOrgID(ctx, u.ID)
	if err != nil {
		return err
	}
	if ownerID != 0 && ownerID != orgID {
		return ErrOrgBotForeignOrg{BotName: u.Name}
	}
	return nil
}

// FindOrgBots returns the bots owned by the organization, ordered by name.
func FindOrgBots(ctx context.Context, orgID int64) ([]*user_model.User, error) {
	bots := make([]*user_model.User, 0, 10)
	return bots, db.GetEngine(ctx).
		Join("INNER", "org_bot", "org_bot.bot_id = `user`.id").
		Where("org_bot.org_id = ?", orgID).
		OrderBy("`user`.lower_name").
		Find(&bots)
}

// GetOrgBotByName returns the bot with the given name if it is owned by the organization.
func GetOrgBotByName(ctx context.Context, orgID int64, name string) (*user_model.User, error) {
	bot, err := user_model.GetUserByName(ctx, name)
	if err != nil {
		return nil, err
	}
	ownerID, err := GetBotOwnerOrgID(ctx, bot.ID)
	if err != nil {
		return nil, err
	}
	if !bot.IsBot() || ownerID != orgID {
		return nil, user_model.ErrUserNotExist{Name: name}
	}
	return bot, nil
}

// CountOrgBots returns the number of bots owned by the organization.
func CountOrgBots(ctx context.Context, orgID int64) (int64, error) {
	return db.GetEngine(ctx).Where("org_id = ?", orgID).Count(new(OrgBot))
}
