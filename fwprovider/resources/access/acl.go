/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

var (
	_ resource.Resource                = &aclResource{}
	_ resource.ResourceWithConfigure   = &aclResource{}
	_ resource.ResourceWithImportState = &aclResource{}
)

var (
	// aclPathRegex is PBS's ACL path format (ACL_PATH_REGEX).
	aclPathRegex = regexp.MustCompile(`^(?:/|(?:/[A-Za-z0-9_][A-Za-z0-9._\-]*)+)$`)
	// aclAuthIDRegex matches a user ID (user@realm) or an API token ID
	// (user@realm!token).
	aclAuthIDRegex = regexp.MustCompile(`^[^\s:/[:cntrl:]]+@[A-Za-z0-9_][A-Za-z0-9._\-]*(?:![A-Za-z0-9_][A-Za-z0-9._\-]*)?$`)
)

// aclRoles are the built-in PBS roles. PBS does not support custom roles.
// The list mirrors `enum Role` in pbs-api-types/src/acl.rs of the upstream
// proxmox repository (https://git.proxmox.com/?p=proxmox.git), in source order.
var aclRoles = []string{
	"Admin",
	"Audit",
	"NoAccess",
	"DatastoreAdmin",
	"DatastoreReader",
	"DatastoreBackup",
	"DatastorePowerUser",
	"DatastoreAudit",
	"RemoteAudit",
	"RemoteAdmin",
	"RemoteSyncOperator",
	"RemoteSyncPushOperator",
	"RemoteDatastorePowerUser",
	"RemoteDatastoreAdmin",
	"TapeAudit",
	"TapeAdmin",
	"TapeOperator",
	"TapeReader",
}

// NewACLResource is a helper function to simplify the provider implementation.
func NewACLResource() resource.Resource {
	return &aclResource{}
}

// aclResource manages one PBS ACL entry.
type aclResource struct {
	client *pbs.Client
}

// aclResourceModel maps the resource schema data.
type aclResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	AuthID    types.String `tfsdk:"auth_id"`
	Role      types.String `tfsdk:"role"`
	Propagate types.Bool   `tfsdk:"propagate"`
}

// aclID builds the resource/import ID "path:auth_id:role". ':' cannot occur
// in any of the three components.
func aclID(aclPath, authID, role string) string {
	return aclPath + ":" + authID + ":" + role
}

// aclParseID splits an ID built by aclID.
func aclParseID(id string) (aclPath, authID, role string, err error) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("expected format <path>:<auth_id>:<role> (e.g. /datastore/store1:backup@pbs:DatastoreBackup), got %q", id)
	}

	return parts[0], parts[1], parts[2], nil
}

// Metadata returns the resource type name.
func (r *aclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

// Schema defines the schema for the resource.
func (r *aclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a single PBS access control list (ACL) entry, granting a role to a user or API token on a path.",
		MarkdownDescription: `Manages a single PBS access control list (ACL) entry, granting a role to a user or API token on a path.

An entry is identified by ` + "`path`, `auth_id` and `role`" + `; changing any of them replaces the entry.
Only ` + "`propagate`" + ` can be updated in place.

**Notes:**

* ` + "`NoAccess`" + ` is exclusive in PBS: assigning it removes all other roles of the same ` + "`auth_id`" + ` on that path,
  and assigning any other role removes an existing ` + "`NoAccess`" + ` entry there. Never combine a ` + "`NoAccess`" + ` entry
  with other ` + "`pbs_acl`" + ` resources for the same ` + "`auth_id`" + ` and path, otherwise they keep removing each other
  and Terraform keeps re-creating them.
* Deleting a user in PBS removes its API tokens and their ACL entries, but not the ACL entries of the user itself.
* Groups are not supported.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Identifier of the ACL entry in the format `<path>:<auth_id>:<role>`.",
				MarkdownDescription: "Identifier of the ACL entry in the format `<path>:<auth_id>:<role>`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Description: "ACL path, e.g. '/', '/datastore/store1', '/datastore/store1/ns1', '/remote/remote1', '/system' or '/tape'. The referenced object need not exist.",
				MarkdownDescription: "ACL path, e.g. `/`, `/datastore/store1`, `/datastore/store1/ns1`, `/remote/remote1`, `/system` or `/tape`. " +
					"The referenced object (e.g. the datastore) need not exist.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(128),
					stringvalidator.RegexMatches(
						aclPathRegex,
						"must be '/' or an absolute path of components that start with an alphanumeric character or underscore and contain only alphanumerics, '_', '.' or '-'",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"auth_id": schema.StringAttribute{
				Description:         "User ID ('user@realm') or API token ID ('user@realm!token') the role is granted to. It must exist.",
				MarkdownDescription: "User ID (`user@realm`) or API token ID (`user@realm!token`) the role is granted to. It must exist.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 64),
					stringvalidator.RegexMatches(
						aclAuthIDRegex,
						"must be in format 'user@realm' or 'user@realm!token'",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description:         "Role to grant. One of: " + strings.Join(aclRoles, ", ") + ".",
				MarkdownDescription: "Role to grant. One of: `" + strings.Join(aclRoles, "`, `") + "`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(aclRoles...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"propagate": schema.BoolAttribute{
				Description:         "Whether the role also applies to all paths below 'path'. Defaults to true.",
				MarkdownDescription: "Whether the role also applies to all paths below `path`. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *aclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

// Create creates the ACL entry.
func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aclPath, authID, role := plan.Path.ValueString(), plan.AuthID.ValueString(), plan.Role.ValueString()

	if err := r.client.Access.SetACL(ctx, aclPath, authID, role, plan.Propagate.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error Creating ACL", "Could not create ACL entry: "+err.Error())
		return
	}

	entry, err := r.client.Access.GetACL(ctx, aclPath, authID, role)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading ACL", "Could not read ACL entry after creation: "+err.Error())
		return
	}

	aclSetState(&plan, entry.Propagate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state from the server.
func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entry, err := r.client.Access.GetACL(ctx, state.Path.ValueString(), state.AuthID.ValueString(), state.Role.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Error Reading ACL", "Could not read ACL entry: "+err.Error())
		return
	}

	aclSetState(&state, entry.Propagate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update changes propagate in place; all other attributes force replacement.
func (r *aclResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aclResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aclPath, authID, role := plan.Path.ValueString(), plan.AuthID.ValueString(), plan.Role.ValueString()

	// Re-adding an existing entry overwrites its propagate flag.
	if err := r.client.Access.SetACL(ctx, aclPath, authID, role, plan.Propagate.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error Updating ACL", "Could not update ACL entry: "+err.Error())
		return
	}

	entry, err := r.client.Access.GetACL(ctx, aclPath, authID, role)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading ACL", "Could not read ACL entry after update: "+err.Error())
		return
	}

	aclSetState(&plan, entry.Propagate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the ACL entry. PBS does not fail if it is already gone.
func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Access.DeleteACL(ctx, state.Path.ValueString(), state.AuthID.ValueString(), state.Role.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting ACL", "Could not delete ACL entry: "+err.Error())
	}
}

// ImportState imports an ACL entry by "<path>:<auth_id>:<role>".
func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	aclPath, authID, role, err := aclParseID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("path"), aclPath)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("auth_id"), authID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role"), role)...)
}

// aclSetState fills the computed attributes from the server values.
func aclSetState(m *aclResourceModel, propagate bool) {
	m.ID = types.StringValue(aclID(m.Path.ValueString(), m.AuthID.ValueString(), m.Role.ValueString()))
	m.Propagate = types.BoolValue(propagate)
}
