/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications

import "github.com/hashicorp/terraform-plugin-framework/attr"

// deleteIfNull appends the PBS property name to deletes when the planned
// value is null. PBS keeps properties that are simply omitted from an update
// request, so clearing an optional attribute requires an explicit "delete".
func deleteIfNull(deletes []string, v attr.Value, property string) []string {
	if v.IsNull() {
		return append(deletes, property)
	}

	return deletes
}
