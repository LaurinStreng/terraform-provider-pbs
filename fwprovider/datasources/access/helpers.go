/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

// Package access provides Terraform data sources for PBS users, ACLs and roles.
package access

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/config"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/access"
)

// configureClient extracts the PBS client from provider data.
func configureClient(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *pbs.Client {
	if req.ProviderData == nil {
		return nil
	}
	cfg, ok := req.ProviderData.(*config.DataSource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *config.DataSource, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}
	return cfg.Client
}

// stringOrNull maps PBS's omitted (empty) strings to null.
func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// userModel holds the attributes of a single user; shared by pbs_user and pbs_users.
type userModel struct {
	UserID    types.String `tfsdk:"userid"`
	Comment   types.String `tfsdk:"comment"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Email     types.String `tfsdk:"email"`
}

// userToModel converts an API user to the data source model. PBS omits
// enable/expire at their defaults (true / 0), so absence maps to those.
func userToModel(u *access.User) userModel {
	m := userModel{
		UserID:    types.StringValue(u.UserID),
		Comment:   stringOrNull(u.Comment),
		Enable:    types.BoolValue(true),
		Expire:    types.Int64Value(0),
		Firstname: stringOrNull(u.Firstname),
		Lastname:  stringOrNull(u.Lastname),
		Email:     stringOrNull(u.Email),
	}
	if u.Enable != nil {
		m.Enable = types.BoolValue(*u.Enable)
	}
	if u.Expire != nil {
		m.Expire = types.Int64Value(*u.Expire)
	}
	return m
}

// userRealm returns the realm part of a userid (after the last '@'; the user
// name may contain '@', the realm may not).
func userRealm(userid string) string {
	i := strings.LastIndex(userid, "@")
	if i < 0 {
		return ""
	}
	return userid[i+1:]
}
