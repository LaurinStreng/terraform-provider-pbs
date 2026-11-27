/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package metrics

import (
	"context"
	"fmt"
	"strings"
	"sync"

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

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/config"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/metrics"
)

// metricsServerMutex is a global mutex to prevent concurrent PBS metrics server operations.
// PBS holds an exclusive lock on /etc/proxmox-backup/.metricserver.lck when writing
// metrics server configurations, which can cause lock contention errors when multiple
// operations happen simultaneously. This mutex ensures operations are serialized.
var metricsServerMutex sync.Mutex

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &metricsServerResource{}
	_ resource.ResourceWithConfigure   = &metricsServerResource{}
	_ resource.ResourceWithImportState = &metricsServerResource{}
)

// NewMetricsServerResource is a helper function to simplify the provider implementation.
func NewMetricsServerResource() resource.Resource {
	return &metricsServerResource{}
}

// metricsServerResource is the resource implementation.
type metricsServerResource struct {
	client *pbs.Client
}

// metricsServerResourceModel maps the resource schema data.
type metricsServerResourceModel struct {
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	URL          types.String `tfsdk:"url"`
	Server       types.String `tfsdk:"server"`
	Port         types.Int64  `tfsdk:"port"`
	Enable       types.Bool   `tfsdk:"enable"`
	MTU          types.Int64  `tfsdk:"mtu"`
	Protocol     types.String `tfsdk:"protocol"`
	Organization types.String `tfsdk:"organization"`
	Bucket       types.String `tfsdk:"bucket"`
	Token        types.String `tfsdk:"token"`
	MaxBodySize  types.Int64  `tfsdk:"max_body_size"`
	VerifyTLS    types.Bool   `tfsdk:"verify_tls"`
	Timeout      types.Int64  `tfsdk:"timeout"`
	Comment      types.String `tfsdk:"comment"`
}

// Metadata returns the resource type name.
func (r *metricsServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_metrics_server"
}

// Schema defines the schema for the resource.
func (r *metricsServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a PBS metrics server configuration for exporting metrics to external monitoring systems.",
		MarkdownDescription: `Manages a PBS metrics server configuration.

Supports InfluxDB (both UDP and HTTP protocols) for metrics export. Use this resource to configure where
PBS should send its performance and usage metrics.`,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description:         "The unique name identifier for the metrics server.",
				MarkdownDescription: "The unique name identifier for the metrics server. This is used to identify the metrics server configuration.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description:         "The type of metrics server.",
				MarkdownDescription: "The type of metrics server. Valid values: `influxdb-udp`, `influxdb-http`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("influxdb-udp", "influxdb-http"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"url": schema.StringAttribute{
				Description:         "Full URL for InfluxDB HTTP (e.g., http://host:8086 or https://host:443). Takes precedence over server+port.",
				MarkdownDescription: "Full URL for InfluxDB HTTP, including protocol (e.g., `http://influxdb:8086` or `https://influxdb:443`). If specified, this takes precedence over separate `server` and `port` fields. Only applicable for `influxdb-http` type.",
				Optional:            true,
			},
			"server": schema.StringAttribute{
				Description:         "The server address (hostname or IP).",
				MarkdownDescription: "The server address (hostname or IP) of the metrics server. Can be used instead of `url` for backwards compatibility.",
				Optional:            true,
			},
			"port": schema.Int64Attribute{
				Description:         "The server port.",
				MarkdownDescription: "The server port. Typical ports: `8089` (InfluxDB UDP), `8086` (InfluxDB HTTP). Can be used instead of `url` for backwards compatibility.",
				Optional:            true,
			},
			"enable": schema.BoolAttribute{
				Description:         "Enable or disable this metrics server.",
				MarkdownDescription: "Enable or disable metrics export to this server. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"mtu": schema.Int64Attribute{
				Description:         "MTU for the metrics connection.",
				MarkdownDescription: "Maximum transmission unit for the metrics connection. Defaults to `1500`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(1500),
			},
			"protocol": schema.StringAttribute{
				Description:         "Protocol for InfluxDB UDP (udp or tcp).",
				MarkdownDescription: "Protocol for InfluxDB UDP connection. Valid values: `udp`, `tcp`. Only applicable for `influxdb-udp` type.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("udp", "tcp"),
				},
			},
			"organization": schema.StringAttribute{
				Description:         "InfluxDB organization (InfluxDB HTTP only).",
				MarkdownDescription: "InfluxDB organization name. Required for `influxdb-http` type.",
				Optional:            true,
			},
			"bucket": schema.StringAttribute{
				Description:         "InfluxDB bucket name (InfluxDB HTTP only).",
				MarkdownDescription: "InfluxDB bucket name where metrics will be stored. Required for `influxdb-http` type.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				Description:         "InfluxDB API token (InfluxDB HTTP only).",
				MarkdownDescription: "InfluxDB API token for authentication. Required for `influxdb-http` type.",
				Optional:            true,
				Sensitive:           true,
			},
			"max_body_size": schema.Int64Attribute{
				Description:         "Maximum body size for HTTP requests in bytes (InfluxDB HTTP only).",
				MarkdownDescription: "Maximum body size for HTTP requests in bytes. Only applicable for `influxdb-http` type. Defaults to `25000000` (25MB).",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(25000000),
			},
			"verify_tls": schema.BoolAttribute{
				Description:         "Verify TLS certificate for HTTPS connections (InfluxDB HTTP only).",
				MarkdownDescription: "Whether to verify TLS certificate for HTTPS connections. Only applicable for `influxdb-http` type. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"timeout": schema.Int64Attribute{
				Description:         "HTTP request timeout in seconds (InfluxDB HTTP only).",
				MarkdownDescription: "HTTP request timeout in seconds. Only applicable for `influxdb-http` type. Defaults to `5`.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(5),
			},
			"comment": schema.StringAttribute{
				Description:         "A comment describing this metrics server.",
				MarkdownDescription: "A comment describing this metrics server configuration.",
				Optional:            true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *metricsServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*config.Resource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *config.Resource, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = cfg.Client
}

// Create creates the resource and sets the initial Terraform state.
func (r *metricsServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan metricsServerResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create metrics server via API
	server := &metrics.MetricsServer{
		Name: plan.Name.ValueString(),
		Type: metrics.MetricsServerType(plan.Type.ValueString()),
	}

	// Set URL or Server+Port
	if !plan.URL.IsNull() {
		server.URL = plan.URL.ValueString()
	} else {
		server.Server = plan.Server.ValueString()
		server.Port = int(plan.Port.ValueInt64())
	}

	// Set optional fields
	if !plan.Enable.IsNull() {
		enable := plan.Enable.ValueBool()
		server.Enable = &enable
	}
	if !plan.MTU.IsNull() {
		mtu := int(plan.MTU.ValueInt64())
		server.MTU = &mtu
	}
	if !plan.Comment.IsNull() {
		server.Comment = plan.Comment.ValueString()
	}

	// Type-specific fields
	switch server.Type {
	case metrics.MetricsServerTypeInfluxDBUDP:
		// PBS 4.0: Protocol field removed (was always "udp" anyway)
	case metrics.MetricsServerTypeInfluxDBHTTP:
		if !plan.Organization.IsNull() {
			server.Organization = plan.Organization.ValueString()
		}
		if !plan.Bucket.IsNull() {
			server.Bucket = plan.Bucket.ValueString()
		}
		if !plan.Token.IsNull() {
			server.Token = plan.Token.ValueString()
		}
		if !plan.MaxBodySize.IsNull() {
			maxBodySize := int(plan.MaxBodySize.ValueInt64())
			server.MaxBodySize = &maxBodySize
		}
		if !plan.VerifyTLS.IsNull() {
			verifyTLS := plan.VerifyTLS.ValueBool()
			server.VerifyTLS = &verifyTLS
		}
		// PBS 4.0: Timeout field removed
	}

	// Acquire mutex to prevent concurrent metrics server operations
	// PBS holds an exclusive lock on the metrics config file
	metricsServerMutex.Lock()
	defer metricsServerMutex.Unlock()

	err := r.client.Metrics.CreateMetricsServer(ctx, server)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating metrics server",
			fmt.Sprintf("Could not create metrics server %s: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	// Set state to fully populated data
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *metricsServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state metricsServerResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get metrics server from API
	serverType := metrics.MetricsServerType(state.Type.ValueString())
	server, err := r.client.Metrics.GetMetricsServer(ctx, serverType, state.Name.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading metrics server",
			fmt.Sprintf("Could not read metrics server %s: %s", state.Name.ValueString(), err.Error()),
		)
		return
	}

	applyServerToState(server, serverType, &state)

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applyServerToState copies the server configuration returned by PBS into
// state. PBS omits values that are at their default, so absent values map to
// the schema defaults. Attributes that are not applicable to the server type
// (or not stored by PBS at all) keep their prior value, falling back to the
// schema default after import.
func applyServerToState(server *metrics.MetricsServer, serverType metrics.MetricsServerType, state *metricsServerResourceModel) {
	// Address: keep the style (url vs. server+port) the user configured.
	// After import neither is set, so url is used for HTTP.
	switch serverType {
	case metrics.MetricsServerTypeInfluxDBHTTP:
		if !state.URL.IsNull() || state.Server.IsNull() {
			state.URL = types.StringValue(server.URL)
			state.Server = types.StringNull()
			state.Port = types.Int64Null()
		} else {
			state.Server = types.StringValue(server.Server)
			state.Port = types.Int64Value(int64(server.Port))
		}
	case metrics.MetricsServerTypeInfluxDBUDP:
		state.Server = types.StringValue(server.Server)
		if server.Port > 0 {
			state.Port = types.Int64Value(int64(server.Port))
		} else {
			state.Port = types.Int64Null()
		}
	}

	state.Enable = boolOrDefault(server.Enable, true)

	if server.Comment != "" {
		state.Comment = types.StringValue(server.Comment)
	} else {
		state.Comment = types.StringNull()
	}

	// Type-specific fields
	switch serverType {
	case metrics.MetricsServerTypeInfluxDBUDP:
		state.MTU = intOrDefault(server.MTU, defaultMTU)
		state.MaxBodySize = keepOrDefaultInt(state.MaxBodySize, defaultMaxBodySize)
		state.VerifyTLS = keepOrDefaultBool(state.VerifyTLS, true)
	case metrics.MetricsServerTypeInfluxDBHTTP:
		state.Organization = stringOrNull(server.Organization)
		state.Bucket = stringOrNull(server.Bucket)
		// The token is never returned by PBS; keep the configured value.
		state.MaxBodySize = intOrDefault(server.MaxBodySize, defaultMaxBodySize)
		state.VerifyTLS = boolOrDefault(server.VerifyTLS, true)
		state.MTU = keepOrDefaultInt(state.MTU, defaultMTU)
	}

	// PBS 4.0 no longer stores a timeout; keep the configured value.
	state.Timeout = keepOrDefaultInt(state.Timeout, defaultTimeout)
}

const (
	defaultMTU         = 1500
	defaultMaxBodySize = 25000000
	defaultTimeout     = 5
)

func stringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func boolOrDefault(v *bool, def bool) types.Bool {
	if v == nil {
		return types.BoolValue(def)
	}
	return types.BoolValue(*v)
}

func intOrDefault(v *int, def int64) types.Int64 {
	if v == nil {
		return types.Int64Value(def)
	}
	return types.Int64Value(int64(*v))
}

func keepOrDefaultInt(v types.Int64, def int64) types.Int64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int64Value(def)
	}
	return v
}

func keepOrDefaultBool(v types.Bool, def bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolValue(def)
	}
	return v
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *metricsServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan metricsServerResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update metrics server via API
	server := &metrics.MetricsServer{
		Name: plan.Name.ValueString(),
	}

	// Set URL or Server+Port
	if !plan.URL.IsNull() {
		server.URL = plan.URL.ValueString()
	} else {
		server.Server = plan.Server.ValueString()
		server.Port = int(plan.Port.ValueInt64())
	}

	// Set optional fields
	if !plan.Enable.IsNull() {
		enable := plan.Enable.ValueBool()
		server.Enable = &enable
	}
	if !plan.MTU.IsNull() {
		mtu := int(plan.MTU.ValueInt64())
		server.MTU = &mtu
	}
	if !plan.Comment.IsNull() {
		server.Comment = plan.Comment.ValueString()
	} else {
		server.Delete = append(server.Delete, "comment")
	}

	// Type-specific fields
	serverType := metrics.MetricsServerType(plan.Type.ValueString())
	switch serverType {
	case metrics.MetricsServerTypeInfluxDBUDP:
		// PBS 4.0: Protocol field removed
	case metrics.MetricsServerTypeInfluxDBHTTP:
		if !plan.Organization.IsNull() {
			server.Organization = plan.Organization.ValueString()
		}
		if !plan.Bucket.IsNull() {
			server.Bucket = plan.Bucket.ValueString()
		}
		if !plan.Token.IsNull() {
			server.Token = plan.Token.ValueString()
		}
		if !plan.MaxBodySize.IsNull() {
			maxBodySize := int(plan.MaxBodySize.ValueInt64())
			server.MaxBodySize = &maxBodySize
		}
		if !plan.VerifyTLS.IsNull() {
			verifyTLS := plan.VerifyTLS.ValueBool()
			server.VerifyTLS = &verifyTLS
		}
		// PBS 4.0: Timeout field removed
	}

	// Acquire mutex to prevent concurrent metrics server operations
	metricsServerMutex.Lock()
	defer metricsServerMutex.Unlock()

	err := r.client.Metrics.UpdateMetricsServer(ctx, serverType, plan.Name.ValueString(), server)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating metrics server",
			fmt.Sprintf("Could not update metrics server %s: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	// Set state
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *metricsServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state metricsServerResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete metrics server via API
	serverType := metrics.MetricsServerType(state.Type.ValueString())

	// Acquire mutex to prevent concurrent metrics server operations
	metricsServerMutex.Lock()
	defer metricsServerMutex.Unlock()

	err := r.client.Metrics.DeleteMetricsServer(ctx, serverType, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting metrics server",
			fmt.Sprintf("Could not delete metrics server %s: %s", state.Name.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports the resource state.
func (r *metricsServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import expects format: type/name (e.g., "influxdb-http/my-server")
	serverType, name, ok := strings.Cut(req.ID, "/")
	if !ok || name == "" ||
		(serverType != string(metrics.MetricsServerTypeInfluxDBHTTP) && serverType != string(metrics.MetricsServerTypeInfluxDBUDP)) {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the format <type>/<name> with type influxdb-http or influxdb-udp, got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), serverType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
