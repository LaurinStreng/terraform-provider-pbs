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

//testacc:tier=medium
//testacc:resource=remotes
func TestAccDataSourceRemoteNamespaces(t *testing.T) {
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
data "pbs_remote_namespaces" "test" {
  remote_name = pbs_remote.test.name
  store       = pbs_datastore.test.name
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.pbs_remote_namespaces.test", "store", "pbs_datastore.test", "name"),
					resource.TestCheckResourceAttr("data.pbs_remote_namespaces.test", "namespaces.#", "2"),
					// The root namespace is reported as an empty string.
					resource.TestCheckTypeSetElemAttr("data.pbs_remote_namespaces.test", "namespaces.*", ""),
					resource.TestCheckTypeSetElemAttr("data.pbs_remote_namespaces.test", "namespaces.*", "tfacc-ns"),
				),
			},
		},
	})
}
