/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

//testacc:tier=medium
//testacc:resource=jobs
func TestAccDataSourceVerifyJob(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	id := te.RandomName("verify")
	te.AddTemplateVars(map[string]any{"ID": id})

	const ds = "data.pbs_verify_job.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id              = "{{.ID}}"
  store           = pbs_datastore.test.name
  schedule        = "daily"
  ignore_verified = true
  outdated_after  = 30
  max_depth       = 3
  namespace       = "tfacc-ns"
  comment         = "read by data source"
}

data "pbs_verify_job" "test" {
  id = pbs_verify_job.test.id
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(ds, "digest", digestRegexp),
					resource.TestCheckResourceAttr(ds, "id", id),
					resource.TestCheckResourceAttr(ds, "store", store),
					resource.TestCheckResourceAttr(ds, "schedule", "daily"),
					resource.TestCheckResourceAttr(ds, "ignore_verified", "true"),
					resource.TestCheckResourceAttr(ds, "outdated_after", "30"),
					resource.TestCheckResourceAttr(ds, "max_depth", "3"),
					resource.TestCheckResourceAttr(ds, "namespace", "tfacc-ns"),
					resource.TestCheckResourceAttr(ds, "comment", "read by data source"),
				),
			},
		},
	})
}
