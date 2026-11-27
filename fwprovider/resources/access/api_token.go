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
	_ resource.Resource                   = &apiTokenResource{}
	_ resource.ResourceWithConfigure      = &apiTokenResource{}
	_ resource.ResourceWithImportState    = &apiTokenResource{}
	_ resource.ResourceWithModifyPlan     = &apiTokenResource{}
	_ resource.ResourceWithValidateConfig = &apiTokenResource{}
)

var (
	apiTokenUserIDRegex = regexp.MustCompile(`^[^\s:/[:cntrl:]]+@[A-Za-z0-9_][A-Za-z0-9._\-]*$`)
	apiTokenNameRegex   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._\-]*$`)
	// apiTokenSingleLineRegex mirrors PBS's SINGLE_LINE_COMMENT_FORMAT.
	apiTokenSingleLineRegex = regexp.MustCompile(`^[^[:cntrl:]]*$`)
)

// apiTokenMaxIDLength is PBS's limit for the full token ID "user@realm!name".
const apiTokenMaxIDLength = 64

// NewAPITokenResource is a helper function to simplify the provider implementation.
func NewAPITokenResource() resource.Resource {
	return &apiTokenResource{}
}

type apiTokenResource struct {
	client *pbs.Client
}

type apiTokenResourceModel struct {
	UserID            types.String `tfsdk:"userid"`
	TokenName         types.String `tfsdk:"token_name"`
	TokenID           types.String `tfsdk:"tokenid"`
	Comment           types.String `tfsdk:"comment"`
	Enable            types.Bool   `tfsdk:"enable"`
	Expire            types.Int64  `tfsdk:"expire"`
	Value             types.String `tfsdk:"value"`
	RegenerateTrigger types.String `tfsdk:"regenerate_trigger"`
}

func (r *apiTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_token"
}

func (r *apiTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an API token of a PBS user.",
		MarkdownDescription: `Manages an API token of a PBS user.

The token secret is returned by PBS only when the token is created (or regenerated) and is stored
in the Terraform state as the sensitive attribute ` + "`value`" + `. Treat the state accordingly.

* To issue a new secret without replacing the token (its ACLs are kept), change ` + "`regenerate_trigger`" + `
  from one non-null value to another. Setting it for the first time (e.g. after import) or removing it
  only records the change and keeps the current secret. Regeneration requires PBS 3.3.7 or later.
* After ` + "`terraform import`" + ` the secret is unknown, so ` + "`value`" + ` is null until the token is regenerated.
* A token only has the privileges granted to it via ACLs (see ` + "`pbs_acl`" + `), intersected with those of its user.
* Deleting the token also deletes its ACL entries; deleting the owning user deletes all of its tokens.`,
		Attributes: map[string]schema.Attribute{
			"userid": schema.StringAttribute{
				Description: "User that owns the token, e.g. `backup@pbs`. Changing this forces a new token.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 64),
					stringvalidator.RegexMatches(apiTokenUserIDRegex, "must be in the format 'user@realm'"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"token_name": schema.StringAttribute{
				Description: "Name of the token (3-64 characters, letters, digits, `_`, `.`, `-`; must not start with `.` or `-`). " +
					"The full token ID `userid!token_name` may not exceed 64 characters. Changing this forces a new token.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 64),
					stringvalidator.RegexMatches(apiTokenNameRegex,
						"must start with a letter, digit or underscore and contain only letters, digits, '_', '.' or '-'"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tokenid": schema.StringAttribute{
				Description: "Full token ID (`userid!token_name`), used as auth ID for ACLs and as API token user.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"comment": schema.StringAttribute{
				Description: "Comment (single line, at most 128 characters).",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 128),
					stringvalidator.RegexMatches(apiTokenSingleLineRegex, "must be a single line without control characters"),
				},
			},
			"enable": schema.BoolAttribute{
				Description: "Whether the token is enabled. Defaults to `true`.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"expire": schema.Int64Attribute{
				Description: "Expiration date as Unix epoch seconds. `0` (default) means the token never expires.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"value": schema.StringAttribute{
				Description: "The token secret. Only known after create or regeneration; null after import.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"regenerate_trigger": schema.StringAttribute{
				Description: "Arbitrary value; when it changes from one non-null value to another, the token secret is " +
					"regenerated in place (the token and its ACLs are kept). Setting it on create, adding it to an " +
					"existing token (e.g. after import) or removing it does not regenerate the secret; the current " +
					"secret is kept and only the new trigger value is recorded.",
				Optional: true,
			},
		},
	}
}

func (r *apiTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c := configureClient(req, resp); c != nil {
		r.client = c
	}
}

// ValidateConfig enforces PBS's limit on the length of the full token ID.
func (r *apiTokenResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg apiTokenResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !isSet(cfg.UserID) || !isSet(cfg.TokenName) {
		return
	}
	tokenID := apiTokenID(cfg.UserID.ValueString(), cfg.TokenName.ValueString())
	if len(tokenID) > apiTokenMaxIDLength {
		resp.Diagnostics.AddAttributeError(path.Root("token_name"), "Token ID Too Long",
			fmt.Sprintf("The full token ID %q is %d characters long; PBS allows at most %d.", tokenID, len(tokenID), apiTokenMaxIDLength))
	}
}

// ModifyPlan marks the secret as unknown when a regeneration is requested
// (see apiTokenRegenerateRequested), so Update can store the new secret.
func (r *apiTokenResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state apiTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A trigger that is unknown at plan time may turn out to differ, so the
	// secret may change as well.
	mayRegenerate := !state.RegenerateTrigger.IsNull() && plan.RegenerateTrigger.IsUnknown()
	if mayRegenerate || apiTokenRegenerateRequested(state.RegenerateTrigger, plan.RegenerateTrigger) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("value"), types.StringUnknown())...)
	}
}

func (r *apiTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid, name := plan.UserID.ValueString(), plan.TokenName.ValueString()
	token := &access.APIToken{Comment: plan.Comment.ValueString()}
	if isSet(plan.Enable) {
		v := plan.Enable.ValueBool()
		token.Enable = &v
	}
	if isSet(plan.Expire) {
		v := plan.Expire.ValueInt64()
		token.Expire = &v
	}

	secret, err := r.client.Access.CreateAPIToken(ctx, userid, name, token)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating API Token",
			fmt.Sprintf("Could not create API token %s: %s", apiTokenID(userid, name), err.Error()))
		return
	}

	// Store the secret right away: PBS never returns it again, so it must not
	// be lost if the read-back below fails.
	plan.TokenID = types.StringValue(apiTokenID(userid, name))
	plan.Value = types.StringValue(secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.Access.GetAPIToken(ctx, userid, name)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading API Token",
			fmt.Sprintf("Could not read API token %s after creation: %s", apiTokenID(userid, name), err.Error()))
		return
	}
	apiTokenSetState(got, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.Access.GetAPIToken(ctx, state.UserID.ValueString(), state.TokenName.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading API Token",
			fmt.Sprintf("Could not read API token %s: %s", state.TokenID.ValueString(), err.Error()))
		return
	}

	apiTokenSetState(got, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state apiTokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userid, name := plan.UserID.ValueString(), plan.TokenName.ValueString()
	tokenID := apiTokenID(userid, name)

	upd := &access.APITokenUpdate{
		Comment: stringPtr(plan.Comment),
		Delete:  deleteIfCleared(nil, "comment", plan.Comment, state.Comment),
	}
	if isSet(plan.Enable) {
		v := plan.Enable.ValueBool()
		upd.Enable = &v
	}
	if isSet(plan.Expire) {
		v := plan.Expire.ValueInt64()
		upd.Expire = &v
	}
	if err := r.client.Access.UpdateAPIToken(ctx, userid, name, upd); err != nil {
		resp.Diagnostics.AddError("Error Updating API Token",
			fmt.Sprintf("Could not update API token %s: %s", tokenID, err.Error()))
		return
	}

	// Keep the known secret unless a regeneration was requested.
	plan.Value = state.Value
	if apiTokenRegenerateRequested(state.RegenerateTrigger, plan.RegenerateTrigger) {
		secret, err := r.client.Access.RegenerateAPIToken(ctx, userid, name)
		if err != nil {
			resp.Diagnostics.AddError("Error Regenerating API Token",
				fmt.Sprintf("Could not regenerate the secret of API token %s: %s", tokenID, err.Error()))
			return
		}
		plan.Value = types.StringValue(secret)
	}

	plan.TokenID = types.StringValue(tokenID)
	got, err := r.client.Access.GetAPIToken(ctx, userid, name)
	if err != nil {
		// Persist a regenerated secret even if the read-back fails.
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError("Error Reading API Token",
			fmt.Sprintf("Could not read API token %s after update: %s", tokenID, err.Error()))
		return
	}
	apiTokenSetState(got, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiTokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Access.DeleteAPIToken(ctx, state.UserID.ValueString(), state.TokenName.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting API Token",
			fmt.Sprintf("Could not delete API token %s: %s", state.TokenID.ValueString(), err.Error()))
	}
}

// ImportState imports a token by its full ID "user@realm!tokenname". The
// secret cannot be read back, so value stays null.
func (r *apiTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userid, name, ok := apiTokenSplitID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid Import ID",
			fmt.Sprintf("Expected an API token ID in the format 'user@realm!tokenname', got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("userid"), userid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token_name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tokenid"), req.ID)...)
}

// apiTokenSetState copies the server-side attributes into the model. userid,
// token_name, value and regenerate_trigger are kept as they are.
func apiTokenSetState(t *access.APIToken, m *apiTokenResourceModel) {
	m.TokenID = types.StringValue(apiTokenID(m.UserID.ValueString(), m.TokenName.ValueString()))
	m.Comment = stringOrNull(t.Comment)
	m.Enable = enableOrDefault(t.Enable)
	m.Expire = expireOrDefault(t.Expire)
}

// apiTokenRegenerateRequested reports whether the secret must be regenerated:
// only when regenerate_trigger changes from one non-null value to another.
// Adding the trigger (null -> value, e.g. after import or on an existing
// token) or removing it (value -> null) just records the new value, so
// clients using the current secret are not silently broken.
func apiTokenRegenerateRequested(prior, planned types.String) bool {
	if prior.IsNull() || prior.IsUnknown() || planned.IsNull() || planned.IsUnknown() {
		return false
	}
	return prior.ValueString() != planned.ValueString()
}

func apiTokenID(userid, name string) string {
	return userid + "!" + name
}

// apiTokenSplitID splits "user@realm!name" at the last '!' (token names
// cannot contain '!').
func apiTokenSplitID(id string) (userid, name string, ok bool) {
	i := strings.LastIndex(id, "!")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	userid, name = id[:i], id[i+1:]
	if !apiTokenUserIDRegex.MatchString(userid) || !apiTokenNameRegex.MatchString(name) {
		return "", "", false
	}
	return userid, name, true
}
