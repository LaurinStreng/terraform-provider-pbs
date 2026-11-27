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
	_ datasource.DataSource              = &aclsDataSource{}
	_ datasource.DataSourceWithConfigure = &aclsDataSource{}
)

// NewACLsDataSource returns the pbs_acls data source.
func NewACLsDataSource() datasource.DataSource {
	return &aclsDataSource{}
}

type aclsDataSource struct {
	client *pbs.Client
}

type aclsDataSourceModel struct {
	Path   types.String `tfsdk:"path"`
	Exact  types.Bool   `tfsdk:"exact"`
	AuthID types.String `tfsdk:"auth_id"`
	ACLs   []aclModel   `tfsdk:"acls"`
}

type aclModel struct {
	Path      types.String `tfsdk:"path"`
	AuthID    types.String `tfsdk:"auth_id"`
	Role      types.String `tfsdk:"role"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

// Metadata returns the data source type name.
func (d *aclsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acls"
}

// Schema defines the schema for the data source.
func (d *aclsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists ACL entries of Proxmox Backup Server for users and API tokens.",
		MarkdownDescription: "Lists ACL entries of Proxmox Backup Server for users and API tokens. " +
			"Group ACL entries are not included.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Description: "Only return entries on this path and, unless exact is true, below it.",
				MarkdownDescription: "Only return entries on this path and, unless `exact` is `true`, below it " +
					"(e.g. `/datastore/store1`). Omit to list all entries.",
				Optional: true,
			},
			"exact": schema.BoolAttribute{
				Description:         "Only return entries on exactly the given path (default false).",
				MarkdownDescription: "Only return entries on exactly `path`, not on sub-paths. Defaults to `false`.",
				Optional:            true,
			},
			"auth_id": schema.StringAttribute{
				Description:         "Only return entries for this user or API token (user@realm or user@realm!token).",
				MarkdownDescription: "Only return entries for this user or API token (`user@realm` or `user@realm!token`).",
				Optional:            true,
			},
			"acls": schema.ListNestedAttribute{
				Description: "List of ACL entries.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"path": schema.StringAttribute{
							Description: "ACL path.",
							Computed:    true,
						},
						"auth_id": schema.StringAttribute{
							Description: "User or API token the entry applies to.",
							Computed:    true,
						},
						"role": schema.StringAttribute{
							Description: "Role granted.",
							Computed:    true,
						},
						"propagate": schema.BoolAttribute{
							Description: "Whether the entry is inherited by sub-paths.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *aclsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		d.client = c
	}
}

// Read lists the ACL entries from PBS.
func (d *aclsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state aclsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries, err := d.client.Access.ListACL(ctx, state.Path.ValueString(), state.Exact.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading ACLs", "Could not list ACLs: "+err.Error())
		return
	}

	authID := state.AuthID.ValueString()
	state.ACLs = make([]aclModel, 0, len(entries))
	for _, e := range entries {
		// Tokens are reported as ugid_type "user" as well; skip groups.
		if e.UGIDType != "user" {
			continue
		}
		if authID != "" && e.UGID != authID {
			continue
		}
		state.ACLs = append(state.ACLs, aclModel{
			Path:      types.StringValue(e.Path),
			AuthID:    types.StringValue(e.UGID),
			Role:      types.StringValue(e.RoleID),
			Propagate: types.BoolValue(e.Propagate),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
