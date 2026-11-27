/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

// Package access implements resources for PBS users, API tokens and ACLs.
package access

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/config"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
)

// configureClient extracts the PBS client from provider data.
func configureClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *pbs.Client {
	if req.ProviderData == nil {
		return nil
	}
	cfg, ok := req.ProviderData.(*config.Resource)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *config.Resource, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}
	return cfg.Client
}

// isSet reports whether a plan/state value is known and non-null.
func isSet(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// stringOrNull maps PBS's omitted (empty) strings to null.
func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// enableOrDefault maps an omitted enable flag to PBS's default (true).
func enableOrDefault(b *bool) types.Bool {
	if b == nil {
		return types.BoolValue(true)
	}
	return types.BoolValue(*b)
}

// expireOrDefault maps an omitted expire value to PBS's default (0 = never).
func expireOrDefault(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Value(0)
	}
	return types.Int64Value(*i)
}

// stringPtr returns a pointer to the value if set, else nil.
func stringPtr(v types.String) *string {
	if !isSet(v) {
		return nil
	}
	s := v.ValueString()
	return &s
}

// deleteIfCleared appends name to deletes when the attribute was set in state
// and is null in the plan, so PBS clears it.
func deleteIfCleared(deletes []string, name string, plan, state types.String) []string {
	if isSet(state) && plan.IsNull() {
		return append(deletes, name)
	}
	return deletes
}
