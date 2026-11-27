/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/config"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &webhookNotificationResource{}
	_ resource.ResourceWithConfigure   = &webhookNotificationResource{}
	_ resource.ResourceWithImportState = &webhookNotificationResource{}
)

// NewWebhookNotificationResource is a helper function to simplify the provider implementation.
func NewWebhookNotificationResource() resource.Resource {
	return &webhookNotificationResource{}
}

// webhookNotificationResource is the resource implementation.
type webhookNotificationResource struct {
	client *pbs.Client
}

// webhookNotificationResourceModel maps the resource schema data.
type webhookNotificationResourceModel struct {
	Name    types.String `tfsdk:"name"`
	URL     types.String `tfsdk:"url"`
	Body    types.String `tfsdk:"body"`
	Method  types.String `tfsdk:"method"`
	Headers types.Map    `tfsdk:"headers"`
	Secret  types.Map    `tfsdk:"secret"`
	Comment types.String `tfsdk:"comment"`
	Disable types.Bool   `tfsdk:"disable"`
	Origin  types.String `tfsdk:"origin"`
}

// Metadata returns the resource type name.
func (r *webhookNotificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_notification"
}

// Schema defines the schema for the resource.
func (r *webhookNotificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Webhook notification target for PBS alerts and notifications.",
		MarkdownDescription: `Manages a Webhook notification target.

Configure a webhook endpoint to receive HTTP notifications from PBS about backup jobs,
verification tasks, and system events.`,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description:         "The unique name identifier for the Webhook target.",
				MarkdownDescription: "The unique name identifier for the Webhook notification target.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Description:         "Webhook URL.",
				MarkdownDescription: "Webhook URL where notifications will be sent (e.g., `https://hooks.example.com/notify`).",
				Required:            true,
			},
			"body": schema.StringAttribute{
				Description:         "Custom request body template.",
				MarkdownDescription: "Custom request body template. Can use template variables for notification data.",
				Optional:            true,
			},
			"method": schema.StringAttribute{
				Description:         "HTTP method for webhook requests.",
				MarkdownDescription: "HTTP method for webhook requests. Valid values: `post`, `put`. Defaults to `post`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("post"),
				Validators: []validator.String{
					stringvalidator.OneOf("post", "put"),
				},
			},
			"headers": schema.MapAttribute{
				Description:         "Custom HTTP headers.",
				MarkdownDescription: "Custom HTTP headers to include in webhook requests. Specify as key-value pairs.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"secret": schema.MapAttribute{
				Description: "Named secrets that can be referenced in the body and header templates as {{ secrets.<name> }}.",
				MarkdownDescription: "Named secrets that can be referenced in the `body` and `headers` templates as " +
					"`{{ secrets.<name> }}`, e.g. `secret = { token = var.webhook_token }` used as " +
					"`{{ secrets.token }}`. PBS stores the values protected and never returns them, so changes made " +
					"to secret values outside Terraform cannot be detected (added or removed secret names are). " +
					"Secret values cannot be imported.",
				ElementType: types.StringType,
				Optional:    true,
				Sensitive:   true,
			},
			"comment": schema.StringAttribute{
				Description:         "A comment describing this notification target.",
				MarkdownDescription: "A comment describing this notification target.",
				Optional:            true,
			},
			"disable": schema.BoolAttribute{
				Description:         "Disable this notification target.",
				MarkdownDescription: "Disable this notification target. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"origin": schema.StringAttribute{
				Description:         "Origin of this configuration as reported by PBS.",
				MarkdownDescription: "Origin of this configuration as reported by PBS (e.g., `user`, `builtin`).",
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *webhookNotificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*config.Resource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *config.Resource, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = config.Client
}

// Create creates the resource and sets the initial Terraform state.
func (r *webhookNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookNotificationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	target := &notifications.WebhookTarget{
		Name: plan.Name.ValueString(),
		URL:  plan.URL.ValueString(),
	}

	if !plan.Body.IsNull() {
		target.Body = plan.Body.ValueString()
	}
	if !plan.Method.IsNull() {
		// PBS 4.0 requires lowercase method values
		target.Method = strings.ToLower(plan.Method.ValueString())
	}
	if !plan.Headers.IsNull() {
		headers := make(map[string]string)
		diags = plan.Headers.ElementsAs(ctx, &headers, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		target.Headers = headers
	}
	if !plan.Secret.IsNull() {
		secrets := make(map[string]string)
		diags = plan.Secret.ElementsAs(ctx, &secrets, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		target.Secrets = secrets
	}
	if !plan.Comment.IsNull() {
		target.Comment = plan.Comment.ValueString()
	}
	if !plan.Disable.IsNull() {
		disable := plan.Disable.ValueBool()
		target.Disable = &disable
	}

	err := r.client.Notifications.CreateWebhookTarget(ctx, target)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Webhook notification target",
			fmt.Sprintf("Could not create Webhook notification target %s: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	created, err := r.client.Notifications.GetWebhookTarget(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading created Webhook notification target",
			fmt.Sprintf("Created Webhook notification target %s but could not read it back: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.URL = types.StringValue(created.URL)

	if created.Body != "" {
		plan.Body = types.StringValue(created.Body)
	} else {
		plan.Body = types.StringNull()
	}

	if created.Method != "" {
		plan.Method = types.StringValue(strings.ToLower(created.Method))
	} else {
		plan.Method = types.StringNull()
	}

	if len(created.Headers) > 0 {
		headersVal, diags := types.MapValueFrom(ctx, types.StringType, created.Headers)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Headers = headersVal
	} else {
		plan.Headers = types.MapNull(types.StringType)
	}

	plan.Secret = reconcileWebhookSecrets(plan.Secret, created.SecretNames)

	if created.Comment != "" {
		plan.Comment = types.StringValue(created.Comment)
	} else {
		plan.Comment = types.StringNull()
	}

	if created.Disable != nil {
		plan.Disable = types.BoolValue(*created.Disable)
	} else {
		plan.Disable = types.BoolValue(false) // PBS omits disable=false
	}

	if created.Origin != "" {
		plan.Origin = types.StringValue(created.Origin)
	} else {
		plan.Origin = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *webhookNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookNotificationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	target, err := r.client.Notifications.GetWebhookTarget(ctx, state.Name.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error reading Webhook notification target",
			fmt.Sprintf("Could not read Webhook notification target %s: %s", state.Name.ValueString(), err.Error()),
		)
		return
	}

	state.URL = types.StringValue(target.URL)
	if target.Body != "" {
		state.Body = types.StringValue(target.Body)
	} else {
		state.Body = types.StringNull()
	}
	if target.Method != "" {
		state.Method = types.StringValue(strings.ToLower(target.Method))
	} else {
		state.Method = types.StringNull()
	}
	if len(target.Headers) > 0 {
		headers, diags := types.MapValueFrom(ctx, types.StringType, target.Headers)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Headers = headers
	} else {
		state.Headers = types.MapNull(types.StringType)
	}
	state.Secret = reconcileWebhookSecrets(state.Secret, target.SecretNames)
	if target.Comment != "" {
		state.Comment = types.StringValue(target.Comment)
	} else {
		state.Comment = types.StringNull()
	}
	if target.Disable != nil {
		state.Disable = types.BoolValue(*target.Disable)
	} else {
		state.Disable = types.BoolValue(false) // PBS omits disable=false
	}

	if target.Origin != "" {
		state.Origin = types.StringValue(target.Origin)
	} else {
		state.Origin = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *webhookNotificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookNotificationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	target := &notifications.WebhookTarget{
		Name: plan.Name.ValueString(),
		URL:  plan.URL.ValueString(),
	}

	if !plan.Body.IsNull() {
		target.Body = plan.Body.ValueString()
	}
	if !plan.Method.IsNull() {
		// PBS 4.0 requires lowercase method values
		target.Method = strings.ToLower(plan.Method.ValueString())
	}
	if !plan.Headers.IsNull() {
		headers := make(map[string]string)
		diags = plan.Headers.ElementsAs(ctx, &headers, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		target.Headers = headers
	}
	if !plan.Secret.IsNull() {
		secrets := make(map[string]string)
		diags = plan.Secret.ElementsAs(ctx, &secrets, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		target.Secrets = secrets
	}
	if !plan.Comment.IsNull() {
		target.Comment = plan.Comment.ValueString()
	}
	if !plan.Disable.IsNull() {
		disable := plan.Disable.ValueBool()
		target.Disable = &disable
	}
	target.Delete = deleteIfNull(target.Delete, plan.Body, "body")
	target.Delete = deleteIfNull(target.Delete, plan.Headers, "header")
	if len(target.Secrets) == 0 {
		// Covers both a removed map and an empty one; omitting "secret" from
		// the update would keep the secrets configured on PBS.
		target.Delete = append(target.Delete, "secret")
	}
	target.Delete = deleteIfNull(target.Delete, plan.Comment, "comment")

	err := r.client.Notifications.UpdateWebhookTarget(ctx, plan.Name.ValueString(), target)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating Webhook notification target",
			fmt.Sprintf("Could not update Webhook notification target %s: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	updated, err := r.client.Notifications.GetWebhookTarget(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading updated Webhook notification target",
			fmt.Sprintf("Updated Webhook notification target %s but could not read it back: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.URL = types.StringValue(updated.URL)

	if updated.Body != "" {
		plan.Body = types.StringValue(updated.Body)
	} else {
		plan.Body = types.StringNull()
	}

	if updated.Method != "" {
		plan.Method = types.StringValue(strings.ToLower(updated.Method))
	} else {
		plan.Method = types.StringNull()
	}

	if len(updated.Headers) > 0 {
		headersVal, diags := types.MapValueFrom(ctx, types.StringType, updated.Headers)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Headers = headersVal
	} else {
		plan.Headers = types.MapNull(types.StringType)
	}

	plan.Secret = reconcileWebhookSecrets(plan.Secret, updated.SecretNames)

	if updated.Comment != "" {
		plan.Comment = types.StringValue(updated.Comment)
	} else {
		plan.Comment = types.StringNull()
	}

	if updated.Disable != nil {
		plan.Disable = types.BoolValue(*updated.Disable)
	} else {
		plan.Disable = types.BoolValue(false) // PBS omits disable=false
	}

	if updated.Origin != "" {
		plan.Origin = types.StringValue(updated.Origin)
	} else {
		plan.Origin = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *webhookNotificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookNotificationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Notifications.DeleteWebhookTarget(ctx, state.Name.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting Webhook notification target",
			fmt.Sprintf("Could not delete Webhook notification target %s: %s", state.Name.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports the resource state.
func (r *webhookNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// reconcileWebhookSecrets builds the secret map from the names PBS reports.
// PBS never returns secret values, so values are kept from prior state; names
// that are new on the server get an empty value and names removed on the
// server disappear, both of which surface as drift.
func reconcileWebhookSecrets(prior types.Map, names []string) types.Map {
	if len(names) == 0 {
		if !prior.IsNull() && !prior.IsUnknown() && len(prior.Elements()) == 0 {
			return prior // keep an explicitly empty map
		}

		return types.MapNull(types.StringType)
	}

	priorElems := prior.Elements()
	elems := make(map[string]attr.Value, len(names))

	for _, n := range names {
		if v, ok := priorElems[n]; ok && !v.IsUnknown() {
			elems[n] = v
		} else {
			elems[n] = types.StringValue("")
		}
	}

	return types.MapValueMust(types.StringType, elems)
}
