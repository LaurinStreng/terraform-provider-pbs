/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package remotes_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// createNamespace creates a namespace in a datastore via the API.
func createNamespace(te *test.Environment, t *testing.T, store, name string) error {
	t.Helper()

	_, err := te.APIClient().Post(te.Context(),
		fmt.Sprintf("/admin/datastore/%s/namespace", url.PathEscape(store)),
		map[string]any{"name": name})

	return err
}

// selfRemoteConfig is a datastore plus a remote pointing back at the same PBS.
const selfRemoteConfig = `
resource "pbs_datastore" "test" {
  name = "{{.Store}}"
  path = "{{.StorePath}}"
}

resource "pbs_remote" "test" {
  name        = "{{.Remote}}"
  host        = "{{.RemoteHost}}"
  auth_id     = "{{.RemoteAuthID}}"
  password    = "{{.RemotePassword}}"
  fingerprint = "{{.RemoteFingerprint}}"
}
`
