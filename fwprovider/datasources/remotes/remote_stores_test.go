/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package remotes_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

//testacc:tier=medium
//testacc:resource=remotes
func TestAccDataSourceRemoteStores(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	store := te.RandomName("ds")
	maintStore := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Store":          store,
		"StorePath":      te.DatastorePath(store),
		"MaintStore":     maintStore,
		"MaintStorePath": te.DatastorePath(maintStore),
		"Remote":         te.RandomName("remote"),
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A datastore in maintenance mode carries extra fields in the
				// scan result and must still be listed.
				Config: te.RenderConfig(selfRemoteConfig + `
resource "pbs_datastore" "maintenance" {
  name = "{{.MaintStore}}"
  path = "{{.MaintStorePath}}"

  maintenance_mode = {
    type    = "read-only"
    message = "tfacc"
  }
}

data "pbs_remote_stores" "test" {
  remote_name = pbs_remote.test.name

  depends_on = [pbs_datastore.test, pbs_datastore.maintenance]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.pbs_remote_stores.test", "remote_name", "pbs_remote.test", "name"),
					resource.TestCheckTypeSetElemAttr("data.pbs_remote_stores.test", "stores.*", store),
					resource.TestCheckTypeSetElemAttr("data.pbs_remote_stores.test", "stores.*", maintStore),
				),
			},
		},
	})
}
