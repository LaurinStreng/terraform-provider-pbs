/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// digestRegexp matches the hex SHA-256 config digest PBS returns.
var digestRegexp = regexp.MustCompile(`^[0-9a-f]{64}$`)

//testacc:tier=medium
//testacc:resource=jobs
func TestAccDataSourcePruneJob(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	id := te.RandomName("prune")
	te.AddTemplateVars(map[string]any{"ID": id})

	const ds = "data.pbs_prune_job.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id          = "{{.ID}}"
  store       = pbs_datastore.test.name
  schedule    = "daily"
  keep_last   = 7
  keep_daily  = 14
  keep_weekly = 4
  max_depth   = 2
  namespace   = "tfacc-ns"
  disable     = true
  comment     = "read by data source"
}

data "pbs_prune_job" "test" {
  id = pbs_prune_job.test.id
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(ds, "digest", digestRegexp),
					resource.TestCheckResourceAttr(ds, "id", id),
					resource.TestCheckResourceAttr(ds, "store", store),
					resource.TestCheckResourceAttr(ds, "schedule", "daily"),
					resource.TestCheckResourceAttr(ds, "keep_last", "7"),
					resource.TestCheckResourceAttr(ds, "keep_daily", "14"),
					resource.TestCheckResourceAttr(ds, "keep_weekly", "4"),
					resource.TestCheckNoResourceAttr(ds, "keep_hourly"),
					resource.TestCheckResourceAttr(ds, "max_depth", "2"),
					resource.TestCheckResourceAttr(ds, "namespace", "tfacc-ns"),
					resource.TestCheckResourceAttr(ds, "disable", "true"),
					resource.TestCheckResourceAttr(ds, "comment", "read by data source"),
				),
			},
		},
	})
}
