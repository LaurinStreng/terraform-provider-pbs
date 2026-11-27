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
func TestAccDataSourceSyncJob(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	store := addDatastoreVars(te)
	id := te.RandomName("sync")
	remote := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"ID": id, "Remote": remote})

	const ds = "data.pbs_sync_job.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "test" {
  id               = "{{.ID}}"
  store            = pbs_datastore.test.name
  remote           = pbs_remote.test.name
  remote_store     = pbs_datastore.test.name
  remote_namespace = "prod"
  namespace        = "mirror"
  schedule         = "hourly"
  max_depth        = 2
  group_filter     = ["group:vm/100", "type:ct"]
  remove_vanished  = true
  comment          = "read by data source"
}

data "pbs_sync_job" "test" {
  id = pbs_sync_job.test.id
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "id", id),
					resource.TestCheckResourceAttr(ds, "store", store),
					resource.TestCheckResourceAttr(ds, "remote", remote),
					resource.TestCheckResourceAttr(ds, "remote_store", store),
					resource.TestCheckResourceAttr(ds, "remote_namespace", "prod"),
					resource.TestCheckResourceAttr(ds, "namespace", "mirror"),
					resource.TestCheckResourceAttr(ds, "schedule", "hourly"),
					resource.TestCheckResourceAttr(ds, "max_depth", "2"),
					resource.TestCheckResourceAttr(ds, "group_filter.#", "2"),
					resource.TestCheckResourceAttr(ds, "group_filter.0", "group:vm/100"),
					resource.TestCheckResourceAttr(ds, "group_filter.1", "type:ct"),
					resource.TestCheckResourceAttr(ds, "remove_vanished", "true"),
					resource.TestCheckResourceAttr(ds, "comment", "read by data source"),
				),
			},
		},
	})
}

// syncLocalBaseConfig is a source and a target datastore on the same PBS for
// local (remote-less) sync jobs.
const syncLocalBaseConfig = `
resource "pbs_datastore" "src" {
  name = "{{.Src}}"
  path = "{{.SrcPath}}"
}

resource "pbs_datastore" "dst" {
  name = "{{.Dst}}"
  path = "{{.DstPath}}"
}
`

func addLocalSyncDatastoreVars(te *test.Environment) (string, string) {
	src := te.RandomName("ds")
	dst := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Src":     src,
		"SrcPath": te.DatastorePath(src),
		"Dst":     dst,
		"DstPath": te.DatastorePath(dst),
	})

	return src, dst
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccDataSourceSyncJobLocal(t *testing.T) {
	te := test.InitEnvironment(t)

	src, dst := addLocalSyncDatastoreVars(te)
	id := te.RandomName("sync")
	te.AddTemplateVars(map[string]any{"ID": id})

	const ds = "data.pbs_sync_job.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "daily"
  comment      = "local -> ext"
}

data "pbs_sync_job" "test" {
  id = pbs_sync_job.test.id
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "id", id),
					resource.TestCheckResourceAttr(ds, "store", dst),
					resource.TestCheckResourceAttr(ds, "remote_store", src),
					resource.TestCheckNoResourceAttr(ds, "remote"),
					resource.TestCheckResourceAttr(ds, "schedule", "daily"),
					resource.TestCheckResourceAttr(ds, "comment", "local -> ext"),
				),
			},
		},
	})
}
