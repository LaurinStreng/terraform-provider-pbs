/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
)

var (
	_ datasource.DataSource              = &usersDataSource{}
	_ datasource.DataSourceWithConfigure = &usersDataSource{}
)

// NewUsersDataSource returns the pbs_users data source.
func NewUsersDataSource() datasource.DataSource {
	return &usersDataSource{}
}

type usersDataSource struct {
	client *pbs.Client
}

type usersDataSourceModel struct {
	Realm types.String `tfsdk:"realm"`
	Users []userModel  `tfsdk:"users"`
}

// Metadata returns the data source type name.
func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

// Schema defines the schema for the data source.
func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Lists users of Proxmox Backup Server.",
		MarkdownDescription: "Lists users of Proxmox Backup Server, optionally filtered by realm. API tokens are not included.",
		Attributes: map[string]schema.Attribute{
			"realm": schema.StringAttribute{
				Description:         "Only return users of this realm (the part after '@', e.g. pbs or pam).",
				MarkdownDescription: "Only return users of this realm (the part after `@`, e.g. `pbs` or `pam`).",
				Optional:            true,
			},
			"users": schema.ListNestedAttribute{
				Description: "List of users.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"userid": schema.StringAttribute{
							Description:         "User ID in the form name@realm.",
							MarkdownDescription: "User ID in the form `name@realm`.",
							Computed:            true,
						},
						"comment": schema.StringAttribute{
							Description: "Comment.",
							Computed:    true,
						},
						"enable": schema.BoolAttribute{
							Description: "Whether the user is enabled.",
							Computed:    true,
						},
						"expire": schema.Int64Attribute{
							Description:         "Account expiration as Unix epoch seconds (0 = never).",
							MarkdownDescription: "Account expiration as Unix epoch seconds (`0` = never).",
							Computed:            true,
						},
						"firstname": schema.StringAttribute{
							Description: "First name.",
							Computed:    true,
						},
						"lastname": schema.StringAttribute{
							Description: "Last name.",
							Computed:    true,
						},
						"email": schema.StringAttribute{
							Description: "E-mail address.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *usersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		d.client = c
	}
}

// Read lists the users from PBS.
func (d *usersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state usersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	users, err := d.client.Access.ListUsers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Users", "Could not list users: "+err.Error())
		return
	}

	realm := state.Realm.ValueString()
	state.Users = make([]userModel, 0, len(users))
	for i := range users {
		if realm != "" && userRealm(users[i].UserID) != realm {
			continue
		}
		state.Users = append(state.Users, userToModel(&users[i]))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
