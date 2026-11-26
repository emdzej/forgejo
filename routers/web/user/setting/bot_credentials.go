// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package setting

import (
	"errors"

	asymkey_model "forgejo.org/models/asymkey"
	auth_model "forgejo.org/models/auth"
	"forgejo.org/models/db"
	"forgejo.org/modules/web"
	asymkey_service "forgejo.org/services/asymkey"
	"forgejo.org/services/context"
	"forgejo.org/services/forms"
)

// LoadBotCredentials loads the access tokens and SSH keys of the bot selected with
// SetCredentialOwner for the shared/bot/credentials template.
func LoadBotCredentials(ctx *context.Context) {
	owner := credentialOwnerOptions(ctx).Owner
	tokens, err := LoadTokensWithResources(ctx, owner)
	if err != nil {
		ctx.ServerError("LoadTokensWithResources", err)
		return
	}
	keys, err := db.Find[asymkey_model.PublicKey](ctx, asymkey_model.FindPublicKeyOptions{
		OwnerID:    owner.ID,
		NotKeytype: asymkey_model.KeyTypePrincipal,
	})
	if err != nil {
		ctx.ServerError("ListPublicKeys", err)
		return
	}
	ctx.Data["TokensWithResources"] = tokens
	ctx.Data["AccessTokenScopePublicOnly"] = string(auth_model.AccessTokenScopePublicOnly)
	ctx.Data["BotKeys"] = keys
	ctx.Data["BotCredentialsLink"] = credentialOwnerOptions(ctx).ActionLink
}

// BotKeyAddPost adds an SSH key to the bot selected with SetCredentialOwner.
func BotKeyAddPost(ctx *context.Context) {
	opts := credentialOwnerOptions(ctx)
	form := web.GetForm(ctx).(*forms.AddKeyForm)
	if ctx.HasError() {
		ctx.Flash.Error(ctx.GetErrMsg())
		ctx.Redirect(opts.ListLink)
		return
	}

	content, err := asymkey_model.CheckPublicKeyString(form.Content)
	if err != nil {
		switch {
		case db.IsErrSSHDisabled(err):
			ctx.Flash.Info(ctx.Tr("settings.ssh_disabled"))
		case asymkey_model.IsErrKeyUnableVerify(err):
			ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
		case errors.Is(err, asymkey_model.ErrKeyIsPrivate):
			ctx.Flash.Error(ctx.Tr("form.must_use_public_key"))
		default:
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
		}
		ctx.Redirect(opts.ListLink)
		return
	}

	if _, err = asymkey_model.AddPublicKey(ctx, opts.Owner.ID, form.Title, content, 0); err != nil {
		switch {
		case asymkey_model.IsErrKeyAlreadyExist(err):
			ctx.Flash.Error(ctx.Tr("settings.ssh_key_been_used"))
		case asymkey_model.IsErrKeyNameAlreadyUsed(err):
			ctx.Flash.Error(ctx.Tr("settings.ssh_key_name_used"))
		case asymkey_model.IsErrKeyUnableVerify(err):
			ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
		default:
			ctx.ServerError("AddPublicKey", err)
			return
		}
		ctx.Redirect(opts.ListLink)
		return
	}
	ctx.Flash.Success(ctx.Tr("settings.add_key_success", form.Title))
	ctx.Redirect(opts.ListLink)
}

// BotKeyDeletePost deletes an SSH key of the bot selected with SetCredentialOwner.
func BotKeyDeletePost(ctx *context.Context) {
	opts := credentialOwnerOptions(ctx)
	if err := asymkey_service.DeletePublicKey(ctx, opts.Owner, ctx.FormInt64("id")); err != nil {
		ctx.Flash.Error("DeletePublicKey: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("settings.ssh_key_deletion_success"))
	}
	ctx.JSONRedirect(opts.ListLink)
}
