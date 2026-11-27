/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

//testacc:tier=medium
//testacc:resource=jobs
func TestAccDataSourceSyncJobs(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	store := addDatastoreVars(te)
	idA := te.RandomName("sync")
	idB := te.RandomName("sync")
	remote := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"IDA": idA, "IDB": idB, "Remote": remote})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "a" {
  id           = "{{.IDA}}"
  store        = pbs_datastore.test.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.test.name
  schedule     = "hourly"
  comment      = "job a"
}

resource "pbs_sync_job" "b" {
  id           = "{{.IDB}}"
  store        = pbs_datastore.test.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.test.name
  schedule     = "daily"
  namespace    = "mirror"
}

data "pbs_sync_jobs" "by_store" {
  store      = pbs_datastore.test.name
  depends_on = [pbs_sync_job.a, pbs_sync_job.b]
}

data "pbs_sync_jobs" "by_remote" {
  remote     = pbs_remote.test.name
  depends_on = [pbs_sync_job.a, pbs_sync_job.b]
}

data "pbs_sync_jobs" "all" {
  depends_on = [pbs_sync_job.a, pbs_sync_job.b]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_sync_jobs.by_store", "jobs.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.by_store", "jobs.*", map[string]string{
						"id": idA, "store": store, "remote": remote, "remote_store": store, "schedule": "hourly", "comment": "job a",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.by_store", "jobs.*", map[string]string{
						"id": idB, "store": store, "remote": remote, "schedule": "daily", "namespace": "mirror",
					}),
					resource.TestCheckResourceAttr("data.pbs_sync_jobs.by_remote", "jobs.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.by_remote", "jobs.*", map[string]string{"id": idA}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.by_remote", "jobs.*", map[string]string{"id": idB}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.all", "jobs.*", map[string]string{"id": idA}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.all", "jobs.*", map[string]string{"id": idB}),
				),
			},
		},
	})
}

// checkSyncJobsListEntry finds the job with the given id in a pbs_sync_jobs
// data source and checks its remote attribute (empty want means null/absent).
func checkSyncJobsListEntry(name, id, wantRemote string, wantPresent bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}

		attrs := rs.Primary.Attributes
		for k, v := range attrs {
			if !strings.HasPrefix(k, "jobs.") || !strings.HasSuffix(k, ".id") || v != id {
				continue
			}

			if !wantPresent {
				return fmt.Errorf("%s: job %s should be filtered out", name, id)
			}

			prefix := strings.TrimSuffix(k, ".id")
			remote, hasRemote := attrs[prefix+".remote"]
			if wantRemote == "" && hasRemote && remote != "" {
				return fmt.Errorf("%s: job %s should have no remote, got %q", name, id, remote)
			}

			if wantRemote != "" && remote != wantRemote {
				return fmt.Errorf("%s: job %s remote = %q, want %q", name, id, remote, wantRemote)
			}

			return nil
		}

		if wantPresent {
			return fmt.Errorf("%s: job %s not found", name, id)
		}

		return nil
	}
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccDataSourceSyncJobsLocal(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	src, dst := addLocalSyncDatastoreVars(te)
	localID := te.RandomName("sync")
	remoteID := te.RandomName("sync")
	remote := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"LocalID": localID, "RemoteID": remoteID, "Remote": remote})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_remote" "test" {
  name        = "{{.Remote}}"
  host        = "{{.RemoteHost}}"
  auth_id     = "{{.RemoteAuthID}}"
  password    = "{{.RemotePassword}}"
  fingerprint = "{{.RemoteFingerprint}}"
}

resource "pbs_sync_job" "local" {
  id           = "{{.LocalID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "daily"
}

resource "pbs_sync_job" "remote" {
  id           = "{{.RemoteID}}"
  store        = pbs_datastore.dst.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.src.name
  schedule     = "daily"
}

data "pbs_sync_jobs" "by_store" {
  store      = pbs_datastore.dst.name
  depends_on = [pbs_sync_job.local, pbs_sync_job.remote]
}

data "pbs_sync_jobs" "by_remote" {
  remote     = pbs_remote.test.name
  depends_on = [pbs_sync_job.local, pbs_sync_job.remote]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_sync_jobs.by_store", "jobs.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_sync_jobs.by_store", "jobs.*", map[string]string{
						"id": localID, "store": dst, "remote_store": src, "schedule": "daily",
					}),
					checkSyncJobsListEntry("data.pbs_sync_jobs.by_store", localID, "", true),
					checkSyncJobsListEntry("data.pbs_sync_jobs.by_store", remoteID, remote, true),
					resource.TestCheckResourceAttr("data.pbs_sync_jobs.by_remote", "jobs.#", "1"),
					checkSyncJobsListEntry("data.pbs_sync_jobs.by_remote", remoteID, remote, true),
					checkSyncJobsListEntry("data.pbs_sync_jobs.by_remote", localID, "", false),
				),
			},
		},
	})
}
