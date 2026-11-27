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
func TestAccDataSourceVerifyJobs(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	idA := te.RandomName("verify")
	idB := te.RandomName("verify")
	te.AddTemplateVars(map[string]any{"IDA": idA, "IDB": idB})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "a" {
  id              = "{{.IDA}}"
  store           = pbs_datastore.test.name
  schedule        = "daily"
  ignore_verified = true
  comment         = "job a"
}

resource "pbs_verify_job" "b" {
  id             = "{{.IDB}}"
  store          = pbs_datastore.test.name
  schedule       = "weekly"
  outdated_after = 14
}

data "pbs_verify_jobs" "by_store" {
  store      = pbs_datastore.test.name
  depends_on = [pbs_verify_job.a, pbs_verify_job.b]
}

data "pbs_verify_jobs" "all" {
  depends_on = [pbs_verify_job.a, pbs_verify_job.b]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_verify_jobs.by_store", "jobs.#", "2"),
					// PBS returns one digest for the whole list; every item carries it.
					resource.TestMatchResourceAttr("data.pbs_verify_jobs.by_store", "jobs.0.digest", digestRegexp),
					resource.TestCheckResourceAttrPair("data.pbs_verify_jobs.by_store", "jobs.0.digest", "data.pbs_verify_jobs.by_store", "jobs.1.digest"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_verify_jobs.by_store", "jobs.*", map[string]string{
						"id": idA, "store": store, "schedule": "daily", "ignore_verified": "true", "comment": "job a",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_verify_jobs.by_store", "jobs.*", map[string]string{
						"id": idB, "store": store, "schedule": "weekly", "outdated_after": "14",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_verify_jobs.all", "jobs.*", map[string]string{"id": idA}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_verify_jobs.all", "jobs.*", map[string]string{"id": idB}),
				),
			},
		},
	})
}
