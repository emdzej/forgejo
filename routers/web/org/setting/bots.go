// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package setting

import (
	"net/http"
	"time"

	"forgejo.org/models"
	"forgejo.org/models/db"
	"forgejo.org/models/organization"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/base"
	"forgejo.org/modules/optional"
	"forgejo.org/modules/web"
	shared_user "forgejo.org/routers/web/shared/user"
	user_setting "forgejo.org/routers/web/user/setting"
	"forgejo.org/services/context"
	"forgejo.org/services/forms"
	user_service "forgejo.org/services/user"
)

const (
	tplBots        base.TplName = "org/settings/bots"
	tplBot         base.TplName = "org/settings/bot"
	tplBotTokenNew base.TplName = "org/settings/bot_token_new"
)

func botsLink(ctx *context.Context) string {
	return ctx.Org.OrgLink + "/settings/bots"
}

func loadBotsData(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings.bots")
	ctx.Data["PageIsSettingsBots"] = true

	bots, err := organization.FindOrgBots(ctx, ctx.Org.Organization.ID)
	if err != nil {
		ctx.ServerError("FindOrgBots", err)
		return
	}
	ctx.Data["Bots"] = bots

	teams, err := organization.FindOrgTeams(ctx, ctx.Org.Organization.ID)
	if err != nil {
		ctx.ServerError("FindOrgTeams", err)
		return
	}
	ctx.Data["Teams"] = teams

	if err := shared_user.LoadHeaderCount(ctx); err != nil {
		ctx.ServerError("LoadHeaderCount", err)
		return
	}
}

// Bots lists the bots of the organization and offers to create one.
func Bots(ctx *context.Context) {
	loadBotsData(ctx)
	if ctx.Written() {
		return
	}
	ctx.HTML(http.StatusOK, tplBots)
}

// BotsCreatePost creates a bot owned by the organization.
func BotsCreatePost(ctx *context.Context) {
	form := web.GetForm(ctx).(*forms.CreateOrgBotForm)
	loadBotsData(ctx)
	if ctx.Written() {
		return
	}
	if ctx.HasError() {
		ctx.HTML(http.StatusOK, tplBots)
		return
	}

	bot, err := user_service.CreateOrgBot(ctx, ctx.Org.Organization, ctx.Doer, user_service.CreateOrgBotOptions{
		Name:     form.UserName,
		FullName: form.FullName,
		TeamIDs:  form.TeamIDs,
	})
	if err != nil {
		ctx.Data["Err_UserName"] = true
		switch {
		case user_model.IsErrUserAlreadyExist(err):
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), tplBots, form)
		case db.IsErrNameReserved(err):
			ctx.RenderWithErr(ctx.Tr("user.form.name_reserved", form.UserName), tplBots, form)
		case db.IsErrNamePatternNotAllowed(err):
			ctx.RenderWithErr(ctx.Tr("user.form.name_pattern_not_allowed", form.UserName), tplBots, form)
		case db.IsErrNameCharsNotAllowed(err):
			ctx.RenderWithErr(ctx.Tr("user.form.name_chars_not_allowed", form.UserName), tplBots, form)
		case user_model.IsErrCooldownPeriod(err):
			ctx.RenderWithErr(ctx.Tr("form.username_claiming_cooldown", err.(user_model.ErrCooldownPeriod).ExpireTime.Format(time.RFC1123Z)), tplBots, form)
		case organization.IsErrTeamNotExist(err):
			ctx.Data["Err_UserName"] = false
			ctx.RenderWithErr(ctx.Tr("form.team_not_exist"), tplBots, form)
		default:
			ctx.ServerError("CreateOrgBot", err)
		}
		return
	}

	ctx.Flash.Success(ctx.Tr("org.settings.bots.created", bot.Name))
	ctx.Redirect(botsLink(ctx) + "/" + bot.Name)
}

// BotAssignment loads the bot named by the {bot} path parameter, checks the doer may
// manage it, and selects it as the owner of the credentials the user_setting handlers manage.
func BotAssignment(ctx *context.Context) {
	bot, err := organization.GetOrgBotByName(ctx, ctx.Org.Organization.ID, ctx.Params(":bot"))
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			ctx.NotFound("GetOrgBotByName", err)
		} else {
			ctx.ServerError("GetOrgBotByName", err)
		}
		return
	}
	if err := user_service.CheckOrgBotManageable(ctx.Doer, bot); err != nil {
		ctx.NotFound("CheckOrgBotManageable", err)
		return
	}

	ctx.Data["Title"] = ctx.Tr("org.settings.bots")
	ctx.Data["PageIsSettingsBots"] = true
	ctx.Data["Bot"] = bot
	link := botsLink(ctx) + "/" + bot.Name
	ctx.Data["BotLink"] = link
	user_setting.SetCredentialOwner(ctx, user_setting.CredentialOwnerOptions{
		Owner:        bot,
		ListLink:     link,
		ActionLink:   link,
		EditTemplate: tplBotTokenNew,
	})
	if err := shared_user.LoadHeaderCount(ctx); err != nil {
		ctx.ServerError("LoadHeaderCount", err)
	}
}

func getBot(ctx *context.Context) *user_model.User {
	return ctx.Data["Bot"].(*user_model.User)
}

// Bot shows a bot of the organization: its profile, credentials and teams.
func Bot(ctx *context.Context) {
	bot := getBot(ctx)
	user_setting.LoadBotCredentials(ctx)
	if ctx.Written() {
		return
	}
	teams, err := organization.GetUserOrgTeams(ctx, ctx.Org.Organization.ID, bot.ID)
	if err != nil {
		ctx.ServerError("GetUserOrgTeams", err)
		return
	}
	ctx.Data["BotTeams"] = teams
	ctx.HTML(http.StatusOK, tplBot)
}

// BotEditPost updates the profile of a bot of the organization.
func BotEditPost(ctx *context.Context) {
	bot := getBot(ctx)
	form := web.GetForm(ctx).(*forms.EditOrgBotForm)
	if ctx.HasError() {
		ctx.Flash.Error(ctx.GetErrMsg())
		ctx.Redirect(ctx.Data["BotLink"].(string))
		return
	}
	if err := user_service.UpdateUser(ctx, bot, &user_service.UpdateOptions{
		FullName:    optional.Some(form.FullName),
		Description: optional.Some(form.Description),
	}); err != nil {
		ctx.ServerError("UpdateUser", err)
		return
	}
	ctx.Flash.Success(ctx.Tr("org.settings.bots.updated"))
	ctx.Redirect(ctx.Data["BotLink"].(string))
}

// BotDeletePost deletes a bot of the organization.
func BotDeletePost(ctx *context.Context) {
	bot := getBot(ctx)
	if err := user_service.DeleteOrgBot(ctx, ctx.Org.Organization, bot); err != nil {
		switch {
		case organization.IsErrLastOrgOwner(err):
			ctx.Flash.Error(ctx.Tr("form.last_org_owner"))
		case models.IsErrUserOwnRepos(err):
			ctx.Flash.Error(ctx.Tr("admin.users.still_own_repo"))
		case models.IsErrUserOwnPackages(err):
			ctx.Flash.Error(ctx.Tr("admin.users.still_own_packages"))
		default:
			ctx.ServerError("DeleteOrgBot", err)
			return
		}
		ctx.JSONRedirect(ctx.Data["BotLink"].(string))
		return
	}
	ctx.Flash.Success(ctx.Tr("org.settings.bots.deleted", bot.Name))
	ctx.JSONRedirect(botsLink(ctx))
}
