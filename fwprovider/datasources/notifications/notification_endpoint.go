/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/config"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &notificationEndpointDataSource{}
	_ datasource.DataSourceWithConfigure = &notificationEndpointDataSource{}
)

// NewNotificationEndpointDataSource is a helper function to simplify the provider implementation.
func NewNotificationEndpointDataSource() datasource.DataSource {
	return &notificationEndpointDataSource{}
}

// notificationEndpointDataSource is the data source implementation.
type notificationEndpointDataSource struct {
	client *pbs.Client
}

// notificationEndpointDataSourceModel maps the data source schema data.
type notificationEndpointDataSourceModel struct {
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Disable types.Bool   `tfsdk:"disable"`
	Comment types.String `tfsdk:"comment"`
	Origin  types.String `tfsdk:"origin"`

	// SMTP fields
	Server     types.String `tfsdk:"server"`
	Port       types.Int64  `tfsdk:"port"`
	From       types.String `tfsdk:"from_address"`
	Mailto     types.List   `tfsdk:"mailto"`
	MailtoUser types.List   `tfsdk:"mailto_user"`
	Mode       types.String `tfsdk:"mode"`
	Username   types.String `tfsdk:"username"`
	Author     types.String `tfsdk:"author"`

	// Gotify/Webhook fields
	URL   types.String `tfsdk:"url"`
	Token types.String `tfsdk:"token"`

	// Webhook fields
	Body        types.String `tfsdk:"body"`
	Method      types.String `tfsdk:"method"`
	Headers     types.Map    `tfsdk:"headers"`
	SecretNames types.List   `tfsdk:"secret_names"`
}

// Metadata returns the data source type name.
func (d *notificationEndpointDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_endpoint"
}

// Schema defines the schema for the data source.
func (d *notificationEndpointDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Reads a notification endpoint configuration from Proxmox Backup Server.",
		MarkdownDescription: "Reads a notification endpoint configuration from Proxmox Backup Server. Supports Gotify, SMTP, Sendmail, and Webhook endpoints.",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description:         "The unique name identifier for the notification endpoint.",
				MarkdownDescription: "The unique name identifier for the notification endpoint.",
				Required:            true,
			},
			"type": schema.StringAttribute{
				Description:         "The type of notification endpoint.",
				MarkdownDescription: "The type of notification endpoint (gotify, smtp, sendmail, or webhook).",
				Required:            true,
			},
			"disable": schema.BoolAttribute{
				Description:         "Whether this endpoint is disabled.",
				MarkdownDescription: "Whether this endpoint is disabled.",
				Computed:            true,
			},
			"comment": schema.StringAttribute{
				Description:         "A comment describing this endpoint.",
				MarkdownDescription: "A comment describing this endpoint.",
				Computed:            true,
			},
			"origin": schema.StringAttribute{
				Description:         "The origin of this endpoint configuration.",
				MarkdownDescription: "The origin of this endpoint configuration (user-api, user-file, etc.).",
				Computed:            true,
			},

			// SMTP/Sendmail fields
			"server": schema.StringAttribute{
				Description:         "SMTP server address (SMTP only).",
				MarkdownDescription: "SMTP server address (SMTP only).",
				Computed:            true,
			},
			"port": schema.Int64Attribute{
				Description:         "SMTP server port (SMTP only).",
				MarkdownDescription: "SMTP server port (SMTP only).",
				Computed:            true,
			},
			"from_address": schema.StringAttribute{
				Description:         "From email address (SMTP/Sendmail only).",
				MarkdownDescription: "From email address (SMTP/Sendmail only).",
				Computed:            true,
			},
			"mailto": schema.ListAttribute{
				Description:         "List of recipient email addresses (SMTP/Sendmail only).",
				MarkdownDescription: "List of recipient email addresses (SMTP/Sendmail only).",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"mailto_user": schema.ListAttribute{
				Description:         "List of PBS user IDs to notify (SMTP/Sendmail only).",
				MarkdownDescription: "List of PBS user IDs to notify (SMTP/Sendmail only).",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"mode": schema.StringAttribute{
				Description:         "SMTP connection mode (SMTP only).",
				MarkdownDescription: "SMTP connection mode: insecure, starttls, or tls (SMTP only).",
				Computed:            true,
			},
			"username": schema.StringAttribute{
				Description:         "SMTP authentication username (SMTP only).",
				MarkdownDescription: "SMTP authentication username (SMTP only).",
				Computed:            true,
			},
			"author": schema.StringAttribute{
				Description:         "Email author/sender name (SMTP/Sendmail only).",
				MarkdownDescription: "Email author/sender name (SMTP/Sendmail only).",
				Computed:            true,
			},

			// Gotify fields
			"token": schema.StringAttribute{
				Description:         "Gotify application token (Gotify only).",
				MarkdownDescription: "Gotify application token (Gotify only). Note: This is sensitive and will not be returned by the API.",
				Computed:            true,
				Sensitive:           true,
			},

			// Gotify/Webhook fields
			"url": schema.StringAttribute{
				Description:         "Target URL (Gotify/Webhook only).",
				MarkdownDescription: "Target URL (Gotify/Webhook only).",
				Computed:            true,
			},

			// Webhook fields
			"secret_names": schema.ListAttribute{
				Description: "Names of the secrets configured for the webhook (Webhook only).",
				MarkdownDescription: "Names of the secrets configured for the webhook, referenced in templates as " +
					"`{{ secrets.<name> }}` (Webhook only). PBS never returns secret values.",
				ElementType: types.StringType,
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description:         "Webhook request body template (Webhook only).",
				MarkdownDescription: "Webhook request body template (Webhook only).",
				Computed:            true,
			},
			"method": schema.StringAttribute{
				Description:         "HTTP method for webhook (Webhook only).",
				MarkdownDescription: "HTTP method for webhook: POST or PUT (Webhook only).",
				Computed:            true,
			},
			"headers": schema.MapAttribute{
				Description:         "Custom HTTP headers for webhook (Webhook only).",
				MarkdownDescription: "Custom HTTP headers for webhook (Webhook only).",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *notificationEndpointDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*config.DataSource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *config.DataSource, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = cfg.Client
}

// Read refreshes the Terraform state with the latest data.
func (d *notificationEndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state notificationEndpointDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	endpointType := state.Type.ValueString()

	// Fetch the appropriate endpoint based on type
	switch notifications.NotificationTargetType(endpointType) {
	case notifications.NotificationTargetTypeGotify:
		target, err := d.client.Notifications.GetGotifyTarget(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Gotify Endpoint",
				fmt.Sprintf("Could not read Gotify endpoint %s: %s", name, err.Error()),
			)
			return
		}
		d.mapGotifyToState(target, &state)

	case notifications.NotificationTargetTypeSMTP:
		target, err := d.client.Notifications.GetSMTPTarget(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading SMTP Endpoint",
				fmt.Sprintf("Could not read SMTP endpoint %s: %s", name, err.Error()),
			)
			return
		}
		d.mapSMTPToState(ctx, target, &state, &resp.Diagnostics)

	case notifications.NotificationTargetTypeSendmail:
		target, err := d.client.Notifications.GetSendmailTarget(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Sendmail Endpoint",
				fmt.Sprintf("Could not read Sendmail endpoint %s: %s", name, err.Error()),
			)
			return
		}
		d.mapSendmailToState(ctx, target, &state, &resp.Diagnostics)

	case notifications.NotificationTargetTypeWebhook:
		target, err := d.client.Notifications.GetWebhookTarget(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Webhook Endpoint",
				fmt.Sprintf("Could not read Webhook endpoint %s: %s", name, err.Error()),
			)
			return
		}
		d.mapWebhookToState(ctx, target, &state, &resp.Diagnostics)

	default:
		resp.Diagnostics.AddError(
			"Invalid Endpoint Type",
			fmt.Sprintf("Unknown endpoint type: %s. Must be one of: gotify, smtp, sendmail, webhook", endpointType),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Helper functions to map API responses to state

func (d *notificationEndpointDataSource) mapGotifyToState(target *notifications.GotifyTarget, state *notificationEndpointDataSourceModel) {
	state.Disable = boolValueOrFalse(target.Disable)
	state.Comment = stringValueOrNull(target.Comment)
	state.Origin = stringValueOrNull(target.Origin)
	state.URL = stringValueOrNull(target.Server)
	// Token is not returned by the API for security reasons
	state.Token = types.StringNull()
}

func (d *notificationEndpointDataSource) mapSMTPToState(ctx context.Context, target *notifications.SMTPTarget, state *notificationEndpointDataSourceModel, diags *diag.Diagnostics) {
	state.Disable = boolValueOrFalse(target.Disable)
	state.Comment = stringValueOrNull(target.Comment)
	state.Origin = stringValueOrNull(target.Origin)
	state.Server = stringValueOrNull(target.Server)
	state.Port = types.Int64Value(int64(smtpPortOrDefault(target.Port, target.Mode)))
	state.From = stringValueOrNull(target.From)
	state.Mode = stringValueOrDefault(target.Mode, "tls")
	state.Username = stringValueOrNull(target.Username)
	state.Author = stringValueOrNull(target.Author)

	// Convert string slices to lists
	if target.To != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, target.To)
		diags.Append(d...)
		state.Mailto = list
	} else {
		state.Mailto = types.ListNull(types.StringType)
	}

	if target.MailtoUser != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, target.MailtoUser)
		diags.Append(d...)
		state.MailtoUser = list
	} else {
		state.MailtoUser = types.ListNull(types.StringType)
	}
}

func (d *notificationEndpointDataSource) mapSendmailToState(ctx context.Context, target *notifications.SendmailTarget, state *notificationEndpointDataSourceModel, diags *diag.Diagnostics) {
	state.Disable = boolValueOrFalse(target.Disable)
	state.Comment = stringValueOrNull(target.Comment)
	state.Origin = stringValueOrNull(target.Origin)
	state.From = stringValueOrNull(target.From)
	state.Author = stringValueOrNull(target.Author)

	if target.Mailto != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, target.Mailto)
		diags.Append(d...)
		state.Mailto = list
	} else {
		state.Mailto = types.ListNull(types.StringType)
	}

	if target.MailtoUser != nil {
		list, d := types.ListValueFrom(ctx, types.StringType, target.MailtoUser)
		diags.Append(d...)
		state.MailtoUser = list
	} else {
		state.MailtoUser = types.ListNull(types.StringType)
	}
}

func (d *notificationEndpointDataSource) mapWebhookToState(ctx context.Context, target *notifications.WebhookTarget, state *notificationEndpointDataSourceModel, diags *diag.Diagnostics) {
	state.Disable = boolValueOrFalse(target.Disable)
	state.Comment = stringValueOrNull(target.Comment)
	state.Origin = stringValueOrNull(target.Origin)
	state.URL = stringValueOrNull(target.URL)
	state.Body = stringValueOrNull(target.Body)
	state.Method = stringValueOrNull(target.Method)
	// PBS returns secret names only, never their values.
	if len(target.SecretNames) > 0 {
		names, d := types.ListValueFrom(ctx, types.StringType, target.SecretNames)
		diags.Append(d...)
		state.SecretNames = names
	} else {
		state.SecretNames = types.ListNull(types.StringType)
	}

	if len(target.Headers) > 0 {
		headers, d := types.MapValueFrom(ctx, types.StringType, target.Headers)
		diags.Append(d...)
		state.Headers = headers
	} else {
		state.Headers = types.MapNull(types.StringType)
	}
}

// Helper functions for null-safe conversions

func stringValueOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// stringValueOrDefault maps an omitted PBS value to its documented default.
func stringValueOrDefault(s, def string) types.String {
	if s == "" {
		return types.StringValue(def)
	}
	return types.StringValue(s)
}

// boolValueOrFalse maps an omitted PBS flag to false (PBS omits false values).
func boolValueOrFalse(ptr *bool) types.Bool {
	return types.BoolValue(ptr != nil && *ptr)
}

// smtpPortOrDefault maps an omitted SMTP port to the PBS default for the mode
// (an omitted mode means "tls").
func smtpPortOrDefault(port *int, mode string) int {
	if port != nil {
		return *port
	}

	switch mode {
	case "insecure":
		return 25
	case "starttls":
		return 587
	default:
		return 465
	}
}
