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

	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
)

var (
	_ datasource.DataSource              = &userDataSource{}
	_ datasource.DataSourceWithConfigure = &userDataSource{}
)

// NewUserDataSource returns the pbs_user data source.
func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client *pbs.Client
}

// Metadata returns the data source type name.
func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema defines the schema for the data source.
func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Reads a single user from Proxmox Backup Server.",
		MarkdownDescription: "Reads a single user from Proxmox Backup Server. Fails if the user does not exist.",
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Description:         "User ID in the form name@realm.",
				MarkdownDescription: "User ID in the form `name@realm` (e.g. `backup@pbs`).",
				Required:            true,
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
	}
}

// Configure adds the provider configured client to the data source.
func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		d.client = c
	}
}

// Read reads the user from PBS.
func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg userModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, err := d.client.Access.GetUser(ctx, cfg.UserID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading User",
			"Could not read user "+cfg.UserID.ValueString()+": "+err.Error(),
		)
		return
	}

	state := userToModel(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
