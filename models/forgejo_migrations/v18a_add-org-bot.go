// Copyright 2026 The Forgejo Authors. All rights reserved.
// SPDX-License-Identifier: GPL-3.0-or-later

package forgejo_migrations

import (
	"forgejo.org/modules/timeutil"

	"code.forgejo.org/xorm/xorm"
)

func init() {
	registerMigration(&Migration{
		Description: "add org_bot table",
		Upgrade:     addOrgBotTable,
	})
}

func addOrgBotTable(x *xorm.Engine) error {
	type OrgBot struct {
		ID          int64              `xorm:"pk autoincr"`
		OrgID       int64              `xorm:"INDEX NOT NULL REFERENCES(user, id)"`
		BotID       int64              `xorm:"UNIQUE NOT NULL REFERENCES(user, id)"`
		CreatedUnix timeutil.TimeStamp `xorm:"created"`
	}

	_, err := x.SyncWithOptions(xorm.SyncOptions{IgnoreDropIndices: true}, new(OrgBot))
	return err
}
