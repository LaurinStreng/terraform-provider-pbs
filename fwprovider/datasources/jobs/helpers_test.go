/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// datastoreConfig is the datastore every job in this package runs against.
const datastoreConfig = `
resource "pbs_datastore" "test" {
  name = "{{.Store}}"
  path = "{{.StorePath}}"
}
`

func addDatastoreVars(te *test.Environment) string {
	store := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Store":     store,
		"StorePath": te.DatastorePath(store),
	})

	return store
}

// syncBaseConfig adds a remote pointing back at the same PBS, so sync jobs
// can pull from the test datastore.
const syncBaseConfig = datastoreConfig + `
resource "pbs_remote" "test" {
  name        = "{{.Remote}}"
  host        = "{{.RemoteHost}}"
  auth_id     = "{{.RemoteAuthID}}"
  password    = "{{.RemotePassword}}"
  fingerprint = "{{.RemoteFingerprint}}"
}
`
