// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package org

import (
	"net/http"

	"forgejo.org/models"
	asymkey_model "forgejo.org/models/asymkey"
	"forgejo.org/models/db"
	"forgejo.org/models/organization"
	user_model "forgejo.org/models/user"
	"forgejo.org/modules/optional"
	api "forgejo.org/modules/structs"
	"forgejo.org/modules/web"
	"forgejo.org/routers/api/v1/user"
	"forgejo.org/routers/api/v1/utils"
	asymkey_service "forgejo.org/services/asymkey"
	"forgejo.org/services/context"
	"forgejo.org/services/convert"
	user_service "forgejo.org/services/user"
)

// BotAssignment loads the organization bot named by the {username} path parameter into
// ctx.User(), checking that the doer may manage it.
func BotAssignment() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		bot, err := organization.GetOrgBotByName(ctx, ctx.Org().Organization.ID, ctx.Params("username"))
		if err != nil {
			if user_model.IsErrUserNotExist(err) {
				ctx.NotFound()
			} else {
				ctx.Error(http.StatusInternalServerError, "GetOrgBotByName", err)
			}
			return
		}
		if err := user_service.CheckOrgBotManageable(ctx.Doer(), bot); err != nil {
			ctx.Error(http.StatusForbidden, "CheckOrgBotManageable", err)
			return
		}
		ctx.SetUser(bot)
	}
}

// ListBots lists the bots of an organization
func ListBots(ctx *context.APIContext) {
	// swagger:operation GET /orgs/{org}/bots organization orgListBots
	// ---
	// summary: List the bots owned by an organization
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/UserList"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	bots, err := organization.FindOrgBots(ctx, ctx.Org().Organization.ID)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "FindOrgBots", err)
		return
	}
	apiBots := make([]*api.User, len(bots))
	for i, bot := range bots {
		apiBots[i] = convert.ToUser(ctx, bot, ctx.Doer())
	}
	ctx.SetTotalCountHeader(int64(len(apiBots)))
	ctx.JSON(http.StatusOK, apiBots)
}

// CreateBot creates a bot owned by an organization
func CreateBot(ctx *context.APIContext) {
	// swagger:operation POST /orgs/{org}/bots organization orgCreateBot
	// ---
	// summary: Create a bot owned by an organization
	// description: Organization bots have no password and cannot sign in to the web UI.
	//   They authenticate with access tokens or SSH keys, are restricted accounts and can
	//   only be members of teams and collaborators on repositories of the organization.
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/CreateOrgBotOption"
	// responses:
	//   "201":
	//     "$ref": "#/responses/User"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/validationError"

	form := web.GetForm(ctx).(*api.CreateOrgBotOption)
	bot, err := user_service.CreateOrgBot(ctx, ctx.Org().Organization, ctx.Doer(), user_service.CreateOrgBotOptions{
		Name:        form.Username,
		FullName:    form.FullName,
		Description: form.Description,
		TeamIDs:     form.TeamIDs,
	})
	if err != nil {
		if user_model.IsErrUserAlreadyExist(err) ||
			db.IsErrNameReserved(err) ||
			db.IsErrNameCharsNotAllowed(err) ||
			db.IsErrNamePatternNotAllowed(err) ||
			user_model.IsErrCooldownPeriod(err) ||
			user_model.IsErrBotUserInvalid(err) ||
			organization.IsErrTeamNotExist(err) {
			ctx.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			ctx.Error(http.StatusInternalServerError, "CreateOrgBot", err)
		}
		return
	}
	ctx.JSON(http.StatusCreated, convert.ToUser(ctx, bot, ctx.Doer()))
}

// GetBot gets a bot of an organization
func GetBot(ctx *context.APIContext) {
	// swagger:operation GET /orgs/{org}/bots/{username} organization orgGetBot
	// ---
	// summary: Get a bot owned by an organization
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/User"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	ctx.JSON(http.StatusOK, convert.ToUser(ctx, ctx.User(), ctx.Doer()))
}

// EditBot edits a bot of an organization
func EditBot(ctx *context.APIContext) {
	// swagger:operation PATCH /orgs/{org}/bots/{username} organization orgEditBot
	// ---
	// summary: Edit a bot owned by an organization
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/EditOrgBotOption"
	// responses:
	//   "200":
	//     "$ref": "#/responses/User"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/validationError"

	form := web.GetForm(ctx).(*api.EditOrgBotOption)
	if err := user_service.UpdateUser(ctx, ctx.User(), &user_service.UpdateOptions{
		FullName:    optional.FromPtr(form.FullName),
		Description: optional.FromPtr(form.Description),
	}); err != nil {
		ctx.Error(http.StatusInternalServerError, "UpdateUser", err)
		return
	}
	ctx.JSON(http.StatusOK, convert.ToUser(ctx, ctx.User(), ctx.Doer()))
}

// DeleteBot deletes a bot of an organization
func DeleteBot(ctx *context.APIContext) {
	// swagger:operation DELETE /orgs/{org}/bots/{username} organization orgDeleteBot
	// ---
	// summary: Delete a bot owned by an organization
	// description: Content created by the bot is kept and attributed to the ghost user.
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// responses:
	//   "204":
	//     "$ref": "#/responses/empty"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/validationError"

	if err := user_service.DeleteOrgBot(ctx, ctx.Org().Organization, ctx.User()); err != nil {
		if organization.IsErrLastOrgOwner(err) ||
			models.IsErrUserOwnRepos(err) ||
			models.IsErrUserOwnPackages(err) {
			ctx.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			ctx.Error(http.StatusInternalServerError, "DeleteOrgBot", err)
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}

// ListBotAccessTokens lists the access tokens of an organization bot
func ListBotAccessTokens(ctx *context.APIContext) {
	// swagger:operation GET /orgs/{org}/bots/{username}/tokens organization orgListBotAccessTokens
	// ---
	// summary: List the access tokens of a bot owned by an organization
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: page
	//   in: query
	//   description: page number of results to return (1-based)
	//   type: integer
	// - name: limit
	//   in: query
	//   description: page size of results
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/AccessTokenList"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	utils.ListAccessTokens(ctx)
}

// CreateBotAccessToken creates an access token for an organization bot
func CreateBotAccessToken(ctx *context.APIContext) {
	// swagger:operation POST /orgs/{org}/bots/{username}/tokens organization orgCreateBotAccessToken
	// ---
	// summary: Create an access token for a bot owned by an organization
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/CreateAccessTokenOption"
	// responses:
	//   "201":
	//     "$ref": "#/responses/AccessToken"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	utils.CreateAccessToken(ctx)
}

// DeleteBotAccessToken deletes an access token of an organization bot
func DeleteBotAccessToken(ctx *context.APIContext) {
	// swagger:operation DELETE /orgs/{org}/bots/{username}/tokens/{id} organization orgDeleteBotAccessToken
	// ---
	// summary: Delete an access token of a bot owned by an organization
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: id
	//   in: path
	//   description: token ID or name
	//   type: string
	//   required: true
	// responses:
	//   "204":
	//     "$ref": "#/responses/empty"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/error"

	utils.DeleteAccessToken(ctx)
}

// ListBotPublicKeys lists the SSH keys of an organization bot
func ListBotPublicKeys(ctx *context.APIContext) {
	// swagger:operation GET /orgs/{org}/bots/{username}/keys organization orgListBotKeys
	// ---
	// summary: List the SSH keys of a bot owned by an organization
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: page
	//   in: query
	//   description: page number of results to return (1-based)
	//   type: integer
	// - name: limit
	//   in: query
	//   description: page size of results
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/PublicKeyList"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	user.ListUserPublicKeys(ctx, ctx.User())
}

// CreateBotPublicKey adds an SSH key to an organization bot
func CreateBotPublicKey(ctx *context.APIContext) {
	// swagger:operation POST /orgs/{org}/bots/{username}/keys organization orgCreateBotKey
	// ---
	// summary: Add an SSH key to a bot owned by an organization
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/CreateKeyOption"
	// responses:
	//   "201":
	//     "$ref": "#/responses/PublicKey"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/validationError"

	form := web.GetForm(ctx).(*api.CreateKeyOption)
	user.CreateUserPublicKey(ctx, *form, ctx.User().ID)
}

// DeleteBotPublicKey deletes an SSH key of an organization bot
func DeleteBotPublicKey(ctx *context.APIContext) {
	// swagger:operation DELETE /orgs/{org}/bots/{username}/keys/{id} organization orgDeleteBotKey
	// ---
	// summary: Delete an SSH key of a bot owned by an organization
	// parameters:
	// - name: org
	//   in: path
	//   description: name of the organization
	//   type: string
	//   required: true
	// - name: username
	//   in: path
	//   description: username of the bot
	//   type: string
	//   required: true
	// - name: id
	//   in: path
	//   description: id of the key to delete
	//   type: integer
	//   format: int64
	//   required: true
	// responses:
	//   "204":
	//     "$ref": "#/responses/empty"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"

	if err := asymkey_service.DeletePublicKey(ctx, ctx.User(), ctx.ParamsInt64(":id")); err != nil {
		switch {
		case asymkey_model.IsErrKeyNotExist(err):
			ctx.NotFound()
		case asymkey_model.IsErrKeyAccessDenied(err):
			ctx.Error(http.StatusForbidden, "", "the key does not belong to this bot")
		default:
			ctx.Error(http.StatusInternalServerError, "DeletePublicKey", err)
		}
		return
	}
	ctx.Status(http.StatusNoContent)
}
