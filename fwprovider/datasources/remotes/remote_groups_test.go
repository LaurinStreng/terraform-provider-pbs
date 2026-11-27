/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package remotes_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// No backups can be created from Terraform, so the scan of a fresh datastore
// must succeed and return an empty list, both for the root and a sub-namespace.
//
//testacc:tier=medium
//testacc:resource=remotes
func TestAccDataSourceRemoteGroups(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	store := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Store":     store,
		"StorePath": te.DatastorePath(store),
		"Remote":    te.RandomName("remote"),
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(selfRemoteConfig),
				Check: func(_ *terraform.State) error {
					return createNamespace(te, t, store, "tfacc-ns")
				},
			},
			{
				Config: te.RenderConfig(selfRemoteConfig + `
data "pbs_remote_groups" "root" {
  remote_name = pbs_remote.test.name
  store       = pbs_datastore.test.name
}

data "pbs_remote_groups" "ns" {
  remote_name = pbs_remote.test.name
  store       = pbs_datastore.test.name
  namespace   = "tfacc-ns"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_remote_groups.root", "groups.#", "0"),
					resource.TestCheckNoResourceAttr("data.pbs_remote_groups.root", "namespace"),
					resource.TestCheckResourceAttr("data.pbs_remote_groups.ns", "groups.#", "0"),
					resource.TestCheckResourceAttr("data.pbs_remote_groups.ns", "namespace", "tfacc-ns"),
				),
			},
		},
	})
}
