/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access

import (
	"context"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/access"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

var (
	_ resource.Resource                = &userResource{}
	_ resource.ResourceWithConfigure   = &userResource{}
	_ resource.ResourceWithImportState    = &userResource{}
	_ resource.ResourceWithValidateConfig = &userResource{}
)

var (
	// userIDRegex mirrors PBS's PROXMOX_USER_ID_REGEX: name@realm.
	userIDRegex = regexp.MustCompile(`^[^\s:/[:cntrl:]]+@[A-Za-z0-9_][A-Za-z0-9._\-]*$`)
	// userSingleLineRegex mirrors PBS's SINGLE_LINE_COMMENT_FORMAT.
	userSingleLineRegex = regexp.MustCompile(`^[^[:cntrl:]]*$`)
)

// NewUserResource returns the pbs_user resource.
func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResource struct {
	client *pbs.Client
}

type userResourceModel struct {
	UserID    types.String `tfsdk:"userid"`
	Comment   types.String `tfsdk:"comment"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Email     types.String `tfsdk:"email"`
	Enable    types.Bool   `tfsdk:"enable"`
	Expire    types.Int64  `tfsdk:"expire"`
	Password  types.String `tfsdk:"password"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func userSingleLineValidators(minLen, maxLen int) []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(minLen, maxLen),
		stringvalidator.RegexMatches(userSingleLineRegex, "must be a single line without control characters"),
	}
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a PBS user.",
		MarkdownDescription: `Manages a Proxmox Backup Server user (` + "`/access/users`" + `).

Users of the ` + "`pbs`" + ` realm can be given a password; users of other realms (e.g. ` + "`pam`" + `, LDAP, OpenID)
authenticate against their realm and only need to exist in PBS for permissions and API tokens.

**Note:** Deleting a user also deletes its API tokens and their ACL entries, but PBS does **not** remove ACL
entries granted to the user itself. Manage those with ` + "`pbs_acl`" + ` resources that reference this user, so
Terraform removes them together with it.

**Note:** The password is write-only: PBS never returns it, so it cannot be imported and changes made outside
Terraform are not detected. Changing the password in place requires the provider to authenticate with
username/password; PBS rejects password changes made with an API token.`,
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Description:         "User ID in the form `name@realm`, e.g. `alice@pbs`. The realm must exist. Changing it replaces the user (and deletes its API tokens).",
				MarkdownDescription: "User ID in the form `name@realm`, e.g. `alice@pbs`. The realm must exist. Changing it replaces the user (and deletes its API tokens).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 64),
					stringvalidator.RegexMatches(userIDRegex, "must be in format 'name@realm'"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Description:         "Comment (single line, at most 128 characters).",
				MarkdownDescription: "Comment (single line, at most 128 characters).",
				Optional:            true,
				Validators:          userSingleLineValidators(1, 128),
			},
			"firstname": schema.StringAttribute{
				Description:         "First name (2-64 characters).",
				MarkdownDescription: "First name (2-64 characters).",
				Optional:            true,
				Validators:          userSingleLineValidators(2, 64),
			},
			"lastname": schema.StringAttribute{
				Description:         "Last name (2-64 characters).",
				MarkdownDescription: "Last name (2-64 characters).",
				Optional:            true,
				Validators:          userSingleLineValidators(2, 64),
			},
			"email": schema.StringAttribute{
				Description:         "E-mail address (2-64 characters).",
				MarkdownDescription: "E-mail address (2-64 characters).",
				Optional:            true,
				Validators:          userSingleLineValidators(2, 64),
			},
			"enable": schema.BoolAttribute{
				Description:         "Whether the user is enabled. Defaults to true.",
				MarkdownDescription: "Whether the user is enabled. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"expire": schema.Int64Attribute{
				Description:         "Account expiration as Unix epoch seconds; 0 means the account never expires. Defaults to 0.",
				MarkdownDescription: "Account expiration as Unix epoch seconds; `0` means the account never expires. Defaults to `0`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"password": schema.StringAttribute{
				Description: "Password (8-64 characters). Only allowed for users of the pbs realm; other realms " +
					"(e.g. pam, LDAP, OpenID) manage passwords themselves. Write-only on the PBS side: " +
					"stored in state, never read back and not importable. Changing it updates the password in place.",
				MarkdownDescription: "Password (8-64 characters). Only allowed for users of the `pbs` realm; other realms " +
					"(e.g. `pam`, LDAP, OpenID) manage passwords themselves, and setting it on a `pam` user would change " +
					"the Linux system password. Write-only on the PBS side: " +
					"stored in state, never read back and not importable. Changing it updates the password in place " +
					"(requires username/password authentication of the provider). Removing it from the configuration " +
					"leaves the current password on the server unchanged.",
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(8, 64),
				},
			},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

// ValidateConfig rejects a password for users outside the pbs realm: for
// pam it would change the Linux system password, and other realms do not
// store passwords in PBS at all.
func (r *userResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config userResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.UserID.IsUnknown() || config.UserID.IsNull() || !isSet(config.Password) {
		return
	}

	userid := config.UserID.ValueString()
	realm := userid[strings.LastIndex(userid, "@")+1:]
	if realm != "pbs" {
		resp.Diagnostics.AddAttributeError(
			path.Root("password"),
			"Invalid Attribute Combination",
			"password can only be set for users of the pbs realm, but "+userid+" belongs to realm \""+realm+
				"\". Users of other realms authenticate against their realm.",
		)
	}
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid := plan.UserID.ValueString()
	user := &access.User{
		UserID:    userid,
		Comment:   plan.Comment.ValueString(),
		Firstname: plan.Firstname.ValueString(),
		Lastname:  plan.Lastname.ValueString(),
		Email:     plan.Email.ValueString(),
	}
	// Only send non-default values: PBS stores create values verbatim.
	if isSet(plan.Enable) && !plan.Enable.ValueBool() {
		enable := false
		user.Enable = &enable
	}
	if isSet(plan.Expire) && plan.Expire.ValueInt64() != 0 {
		expire := plan.Expire.ValueInt64()
		user.Expire = &expire
	}

	password := ""
	if isSet(plan.Password) {
		password = plan.Password.ValueString()
	}

	// PBS saves the user before storing the password, so a failed password
	// store leaves a user behind. Remember whether the user already existed
	// so cleanup never deletes a user Terraform did not create.
	existedBefore := false
	if password != "" {
		_, getErr := r.client.Access.GetUser(ctx, userid)
		existedBefore = getErr == nil
	}

	if err := r.client.Access.CreateUser(ctx, user, password); err != nil {
		detail := "Could not create user " + userid + ": " + err.Error()
		if password != "" && !existedBefore {
			r.cleanupPartialUser(ctx, userid, &detail)
		}
		resp.Diagnostics.AddError("Error Creating User", detail)
		return
	}

	created, err := r.client.Access.GetUser(ctx, userid)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading User", "Could not read user "+userid+" after creation: "+err.Error())
		return
	}

	state := userModelFromAPI(created, plan.Password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// cleanupPartialUser best-effort deletes a user left behind by a failed
// create and appends a note to detail if that fails.
func (r *userResource) cleanupPartialUser(ctx context.Context, userid string, detail *string) {
	if _, err := r.client.Access.GetUser(ctx, userid); err != nil {
		return
	}
	if err := r.client.Access.DeleteUser(ctx, userid); err != nil {
		*detail += "\n\nThe user was partially created and could not be removed: " + err.Error()
	}
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid := state.UserID.ValueString()
	user, err := r.client.Access.GetUser(ctx, userid)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading User", "Could not read user "+userid+": "+err.Error())
		return
	}

	// The password is never returned by PBS; keep what is in state.
	newState := userModelFromAPI(user, state.Password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid := plan.UserID.ValueString()
	upd := &access.UserUpdate{
		Comment:   stringPtr(plan.Comment),
		Firstname: stringPtr(plan.Firstname),
		Lastname:  stringPtr(plan.Lastname),
		Email:     stringPtr(plan.Email),
	}
	// enable/expire cannot be deleted; sending the defaults (true / 0)
	// makes PBS drop them from the config.
	if isSet(plan.Enable) {
		enable := plan.Enable.ValueBool()
		upd.Enable = &enable
	}
	if isSet(plan.Expire) {
		expire := plan.Expire.ValueInt64()
		upd.Expire = &expire
	}
	upd.Delete = deleteIfCleared(upd.Delete, "comment", plan.Comment, state.Comment)
	upd.Delete = deleteIfCleared(upd.Delete, "firstname", plan.Firstname, state.Firstname)
	upd.Delete = deleteIfCleared(upd.Delete, "lastname", plan.Lastname, state.Lastname)
	upd.Delete = deleteIfCleared(upd.Delete, "email", plan.Email, state.Email)

	if err := r.client.Access.UpdateUser(ctx, userid, upd); err != nil {
		resp.Diagnostics.AddError("Error Updating User", "Could not update user "+userid+": "+err.Error())
		return
	}

	// PUT /access/users ignores passwords; a changed password needs the
	// dedicated endpoint. Removing the password from the config leaves the
	// server-side password untouched.
	password := state.Password
	if isSet(plan.Password) && plan.Password.ValueString() != state.Password.ValueString() {
		if err := r.client.Access.ChangePassword(ctx, userid, plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error Updating User", "Could not change password of user "+userid+": "+err.Error())
		} else {
			password = plan.Password
		}
	} else {
		password = plan.Password
	}

	updated, err := r.client.Access.GetUser(ctx, userid)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading User", "Could not read user "+userid+" after update: "+err.Error())
		return
	}

	newState := userModelFromAPI(updated, password)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid := state.UserID.ValueString()
	if err := r.client.Access.DeleteUser(ctx, userid); err != nil {
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting User", "Could not delete user "+userid+": "+err.Error())
	}
}

// ImportState imports a user by its userid (e.g. "alice@pbs"). The password
// cannot be imported.
func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("userid"), req, resp)
}

// userModelFromAPI maps a PBS user to the resource model, mapping omitted
// values to PBS defaults. password is carried over since PBS never returns it.
func userModelFromAPI(u *access.User, password types.String) userResourceModel {
	return userResourceModel{
		UserID:    types.StringValue(u.UserID),
		Comment:   stringOrNull(u.Comment),
		Firstname: stringOrNull(u.Firstname),
		Lastname:  stringOrNull(u.Lastname),
		Email:     stringOrNull(u.Email),
		Enable:    enableOrDefault(u.Enable),
		Expire:    expireOrDefault(u.Expire),
		Password:  password,
	}
}
