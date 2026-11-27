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
	_ datasource.DataSource              = &rolesDataSource{}
	_ datasource.DataSourceWithConfigure = &rolesDataSource{}
)

// NewRolesDataSource returns the pbs_roles data source.
func NewRolesDataSource() datasource.DataSource {
	return &rolesDataSource{}
}

type rolesDataSource struct {
	client *pbs.Client
}

type rolesDataSourceModel struct {
	Roles []roleModel `tfsdk:"roles"`
}

type roleModel struct {
	RoleID     types.String   `tfsdk:"role_id"`
	Privileges []types.String `tfsdk:"privileges"`
	Comment    types.String   `tfsdk:"comment"`
}

// Metadata returns the data source type name.
func (d *rolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

// Schema defines the schema for the data source.
func (d *rolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Lists the built-in roles of Proxmox Backup Server and their privileges.",
		MarkdownDescription: "Lists the built-in roles of Proxmox Backup Server and their privileges. PBS roles are fixed and cannot be created.",
		Attributes: map[string]schema.Attribute{
			"roles": schema.ListNestedAttribute{
				Description: "List of roles.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role_id": schema.StringAttribute{
							Description:         "Role name, usable as role of pbs_acl.",
							MarkdownDescription: "Role name, usable as `role` of `pbs_acl` (e.g. `DatastoreBackup`).",
							Computed:            true,
						},
						"privileges": schema.ListAttribute{
							Description: "Privileges granted by the role.",
							ElementType: types.StringType,
							Computed:    true,
						},
						"comment": schema.StringAttribute{
							Description: "Description of the role.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *rolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		d.client = c
	}
}

// Read lists the roles from PBS.
func (d *rolesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	roles, err := d.client.Access.ListRoles(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Roles", "Could not list roles: "+err.Error())
		return
	}

	state := rolesDataSourceModel{Roles: make([]roleModel, 0, len(roles))}
	for _, r := range roles {
		privs := make([]types.String, 0, len(r.Privs))
		for _, p := range r.Privs {
			privs = append(privs, types.StringValue(p))
		}
		state.Roles = append(state.Roles, roleModel{
			RoleID:     types.StringValue(r.RoleID),
			Privileges: privs,
			Comment:    stringOrNull(r.Comment),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
