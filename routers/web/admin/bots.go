// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package admin

import (
	"strconv"

	"forgejo.org/models/organization"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/base"
	"forgejo.org/modules/setting"
	user_setting "forgejo.org/routers/web/user/setting"
	"forgejo.org/services/context"
)

const tplBotTokenNew base.TplName = "admin/user/bot_token_new"

func setBotCredentialOwner(ctx *context.Context, bot *user_model.User) {
	userLink := setting.AppSubURL + "/admin/users/" + strconv.FormatInt(bot.ID, 10)
	user_setting.SetCredentialOwner(ctx, user_setting.CredentialOwnerOptions{
		Owner:        bot,
		ListLink:     userLink + "/edit",
		ActionLink:   userLink,
		EditTemplate: tplBotTokenNew,
	})
}

// loadBotInfo loads what the edit page shows about bots: the organization owning the bot
// and its credentials.
func loadBotInfo(ctx *context.Context, bot *user_model.User) {
	ownerID, err := organization.GetBotOwnerOrgID(ctx, bot.ID)
	if err != nil {
		ctx.ServerError("GetBotOwnerOrgID", err)
		return
	}
	if ownerID != 0 {
		org, err := organization.GetOrgByID(ctx, ownerID)
		if err != nil {
			ctx.ServerError("GetOrgByID", err)
			return
		}
		ctx.Data["BotOwnerOrg"] = org
	}
	setBotCredentialOwner(ctx, bot)
	user_setting.LoadBotCredentials(ctx)
}

// BotCredentialsAssignment selects the bot whose credentials the user_setting handlers manage.
func BotCredentialsAssignment(ctx *context.Context) {
	bot, err := user_model.GetUserByID(ctx, ctx.ParamsInt64(":userid"))
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			ctx.NotFound("GetUserByID", err)
		} else {
			ctx.ServerError("GetUserByID", err)
		}
		return
	}
	if !bot.IsBot() {
		ctx.NotFound("not a bot", nil)
		return
	}
	ctx.Data["Title"] = ctx.Tr("admin.users.edit_account")
	ctx.Data["PageIsAdminUsers"] = true
	ctx.Data["User"] = bot
	setBotCredentialOwner(ctx, bot)
}
