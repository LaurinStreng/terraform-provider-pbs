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
func TestAccDataSourcePruneJobs(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	idA := te.RandomName("prune")
	idB := te.RandomName("prune")
	te.AddTemplateVars(map[string]any{"IDA": idA, "IDB": idB})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "a" {
  id        = "{{.IDA}}"
  store     = pbs_datastore.test.name
  schedule  = "daily"
  keep_last = 3
  comment   = "job a"
}

resource "pbs_prune_job" "b" {
  id         = "{{.IDB}}"
  store      = pbs_datastore.test.name
  schedule   = "weekly"
  keep_daily = 7
}

data "pbs_prune_jobs" "by_store" {
  store      = pbs_datastore.test.name
  depends_on = [pbs_prune_job.a, pbs_prune_job.b]
}

data "pbs_prune_jobs" "all" {
  depends_on = [pbs_prune_job.a, pbs_prune_job.b]
}`),
				Check: resource.ComposeTestCheckFunc(
					// The store filter returns exactly the jobs of this datastore.
					resource.TestCheckResourceAttr("data.pbs_prune_jobs.by_store", "jobs.#", "2"),
					// PBS returns one digest for the whole list; every item carries it.
					resource.TestMatchResourceAttr("data.pbs_prune_jobs.by_store", "jobs.0.digest", digestRegexp),
					resource.TestCheckResourceAttrPair("data.pbs_prune_jobs.by_store", "jobs.0.digest", "data.pbs_prune_jobs.by_store", "jobs.1.digest"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_prune_jobs.by_store", "jobs.*", map[string]string{
						"id": idA, "store": store, "schedule": "daily", "keep_last": "3", "comment": "job a",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_prune_jobs.by_store", "jobs.*", map[string]string{
						"id": idB, "store": store, "schedule": "weekly", "keep_daily": "7",
					}),
					// Without a filter, other jobs may exist too, but ours are included.
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_prune_jobs.all", "jobs.*", map[string]string{"id": idA}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_prune_jobs.all", "jobs.*", map[string]string{"id": idB}),
				),
			},
		},
	})
}
