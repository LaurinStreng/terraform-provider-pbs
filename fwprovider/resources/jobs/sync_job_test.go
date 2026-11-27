/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/jobs"
)

const syncJobResourceName = "pbs_sync_job.test"

// syncBaseConfig is a datastore plus a remote pointing back at the same PBS;
// the sync job pulls from that datastore via the remote.
const syncBaseConfig = datastoreConfig + `
resource "pbs_remote" "test" {
  name        = "{{.Remote}}"
  host        = "{{.RemoteHost}}"
  auth_id     = "{{.RemoteAuthID}}"
  password    = "{{.RemotePassword}}"
  fingerprint = "{{.RemoteFingerprint}}"
}
`

func checkSyncJobDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_sync_job", "id", func(id string) error {
		_, err := te.Client().Jobs.GetSyncJob(te.Context(), id)
		return err
	})
}

func checkSyncJobOnServer(te *test.Environment, id string, fn func(*jobs.SyncJob) error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		job, err := te.Client().Jobs.GetSyncJob(te.Context(), id)
		if err != nil {
			return fmt.Errorf("reading sync job %q from API: %w", id, err)
		}

		return fn(job)
	}
}

func isTrue(b *bool) bool { return b != nil && *b }

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceSyncJob(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	store := addDatastoreVars(te)
	remote := te.RandomName("remote")
	id := te.RandomName("sync")
	newID := te.RandomName("sync")
	te.AddTemplateVars(map[string]any{"ID": id, "NewID": newID, "Remote": remote})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkSyncJobDestroy(te),
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
  remove_vanished  = true
  resync_corrupt   = true
  rate_in          = "10M"
  rate_out         = "5MiB"
  burst_in         = "15M"
  burst_out        = "10M"
  comment          = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "id", id),
					resource.TestCheckResourceAttr(syncJobResourceName, "store", store),
					resource.TestCheckResourceAttr(syncJobResourceName, "remote", remote),
					resource.TestCheckResourceAttr(syncJobResourceName, "remote_store", store),
					resource.TestCheckResourceAttr(syncJobResourceName, "remote_namespace", "prod"),
					resource.TestCheckResourceAttr(syncJobResourceName, "namespace", "mirror"),
					resource.TestCheckResourceAttr(syncJobResourceName, "schedule", "hourly"),
					resource.TestCheckResourceAttr(syncJobResourceName, "remove_vanished", "true"),
					resource.TestCheckResourceAttr(syncJobResourceName, "resync_corrupt", "true"),
					resource.TestCheckResourceAttr(syncJobResourceName, "verified_only", "false"),
					resource.TestCheckResourceAttr(syncJobResourceName, "rate_in", "10M"),
					// Equivalent spellings of a byte size must be kept as written
					// (PBS returns its own normalized form); the post-apply plan must be empty.
					resource.TestCheckResourceAttr(syncJobResourceName, "rate_out", "5MiB"),
					resource.TestCheckResourceAttr(syncJobResourceName, "burst_out", "10M"),
					checkSyncJobOnServer(te, id, func(j *jobs.SyncJob) error {
						if j.Store != store || j.Remote != remote || j.RemoteStore != store ||
							j.RemoteNamespace != "prod" || j.Namespace != "mirror" || j.Schedule != "hourly" ||
							j.Comment != "created by acceptance test" {
							return fmt.Errorf("unexpected job on server: %+v", j)
						}

						if !isTrue(j.RemoveVanished) || !isTrue(j.ResyncCorrupt) {
							return fmt.Errorf("remove-vanished/resync-corrupt not set on server: %+v", j)
						}

						if j.RateIn == "" || j.RateOut == "" || j.BurstIn == "" || j.BurstOut == "" {
							return fmt.Errorf("rate limits not set on server: %+v", j)
						}

						return nil
					}),
				),
			},
			{
				Config: te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "test" {
  id               = "{{.ID}}"
  store            = pbs_datastore.test.name
  remote           = pbs_remote.test.name
  remote_store     = pbs_datastore.test.name
  remote_namespace = "prod"
  namespace        = "mirror"
  schedule         = "daily"
  remove_vanished  = false
  verified_only    = true
  run_on_mount     = true
  encrypted_only   = true
  transfer_last    = 3600
  max_depth        = 2
  owner            = "{{.RemoteAuthID}}"
  group_filter     = ["group:vm/100", "type:ct"]
  sync_direction   = "pull"
  rate_in          = "20M"
  rate_out         = "8M"
  burst_in         = "25M"
  burst_out        = "12M"
  comment          = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "schedule", "daily"),
					resource.TestCheckResourceAttr(syncJobResourceName, "remove_vanished", "false"),
					resource.TestCheckResourceAttr(syncJobResourceName, "verified_only", "true"),
					resource.TestCheckResourceAttr(syncJobResourceName, "run_on_mount", "true"),
					resource.TestCheckResourceAttr(syncJobResourceName, "encrypted_only", "true"),
					resource.TestCheckResourceAttr(syncJobResourceName, "transfer_last", "3600"),
					resource.TestCheckResourceAttr(syncJobResourceName, "max_depth", "2"),
					resource.TestCheckResourceAttr(syncJobResourceName, "group_filter.#", "2"),
					resource.TestCheckResourceAttr(syncJobResourceName, "group_filter.0", "group:vm/100"),
					resource.TestCheckResourceAttr(syncJobResourceName, "group_filter.1", "type:ct"),
					resource.TestCheckResourceAttr(syncJobResourceName, "rate_in", "20M"),
					resource.TestCheckResourceAttr(syncJobResourceName, "sync_direction", "pull"),
					resource.TestCheckResourceAttr(syncJobResourceName, "burst_out", "12M"),
					checkSyncJobOnServer(te, id, func(j *jobs.SyncJob) error {
						if j.Schedule != "daily" || j.Comment != "updated by acceptance test" {
							return fmt.Errorf("unexpected job on server: %+v", j)
						}

						if isTrue(j.RemoveVanished) || !isTrue(j.VerifiedOnly) || !isTrue(j.RunOnMount) || !isTrue(j.EncryptedOnly) {
							return fmt.Errorf("unexpected flags on server: %+v", j)
						}

						if intPtrString(j.TransferLast) != "3600" || intPtrString(j.MaxDepth) != "2" {
							return fmt.Errorf("transfer-last=%s max-depth=%s, want 3600/2",
								intPtrString(j.TransferLast), intPtrString(j.MaxDepth))
						}

						if !slices.Equal(j.GroupFilter, []string{"group:vm/100", "type:ct"}) {
							return fmt.Errorf("group-filter = %v", j.GroupFilter)
						}

						if j.Owner == "" {
							return fmt.Errorf("owner not set on server")
						}

						return nil
					}),
				),
			},
			{
				ResourceName:                         syncJobResourceName,
				ImportState:                          true,
				ImportStateId:                        id,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.test.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.test.name
  schedule     = "daily"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(syncJobResourceName, "remote_namespace"),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "namespace"),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "group_filter"),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "rate_in"),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "comment"),
					resource.TestCheckResourceAttr(syncJobResourceName, "verified_only", "false"),
					checkSyncJobOnServer(te, id, func(j *jobs.SyncJob) error {
						if j.RemoteNamespace != "" || j.Namespace != "" || len(j.GroupFilter) != 0 ||
							j.RateIn != "" || j.RateOut != "" || j.BurstIn != "" || j.BurstOut != "" ||
							j.Comment != "" || j.TransferLast != nil || j.MaxDepth != nil || j.Owner != "" {
							return fmt.Errorf("optional fields still set on server: %+v", j)
						}

						if isTrue(j.VerifiedOnly) || isTrue(j.RunOnMount) || isTrue(j.EncryptedOnly) {
							return fmt.Errorf("flags still set on server: %+v", j)
						}

						return nil
					}),
				),
			},
			{
				// The job ID is immutable and forces replacement.
				Config: te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.NewID}}"
  store        = pbs_datastore.test.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.test.name
  schedule     = "daily"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "id", newID),
					checkSyncJobOnServer(te, newID, func(*jobs.SyncJob) error { return nil }),
					func(_ *terraform.State) error {
						_, err := te.Client().Jobs.GetSyncJob(te.Context(), id)
						if !test.IsNotFound(err) {
							return fmt.Errorf("old sync job %q should be gone, got err=%v", id, err)
						}

						return nil
					},
				),
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceSyncJobDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	addDatastoreVars(te)
	id := te.RandomName("sync")
	te.AddTemplateVars(map[string]any{"ID": id, "Remote": te.RandomName("remote")})

	config := te.RenderConfig(syncBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.test.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.test.name
  schedule     = "daily"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkSyncJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the job behind Terraform's back.
					return te.Client().Jobs.DeleteSyncJob(te.Context(), id)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionCreate),
					},
				},
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

// checkLocalSyncJobOnServer asserts the job on PBS is a local sync job
// (no remote) from src into dst.
func checkLocalSyncJobOnServer(te *test.Environment, id, src, dst string, fn func(*jobs.SyncJob) error) resource.TestCheckFunc {
	return checkSyncJobOnServer(te, id, func(j *jobs.SyncJob) error {
		if j.Remote != "" || j.RemoteStore != src || j.Store != dst {
			return fmt.Errorf("expected local sync %s -> %s without remote, got %+v", src, dst, j)
		}

		if fn != nil {
			return fn(j)
		}

		return nil
	})
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceSyncJobLocal(t *testing.T) {
	te := test.InitEnvironment(t)

	src, dst := addLocalSyncDatastoreVars(te)
	id := te.RandomName("sync")
	te.AddTemplateVars(map[string]any{"ID": id})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkSyncJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_sync_job" "test" {
  id              = "{{.ID}}"
  store           = pbs_datastore.dst.name
  remote_store    = pbs_datastore.src.name
  schedule        = "daily"
  remove_vanished = true
  comment         = "local -> ext"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "id", id),
					resource.TestCheckResourceAttr(syncJobResourceName, "store", dst),
					resource.TestCheckResourceAttr(syncJobResourceName, "remote_store", src),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "remote"),
					resource.TestCheckResourceAttr(syncJobResourceName, "remove_vanished", "true"),
					resource.TestCheckResourceAttrSet(syncJobResourceName, "digest"),
					checkLocalSyncJobOnServer(te, id, src, dst, func(j *jobs.SyncJob) error {
						if j.Schedule != "daily" || j.Comment != "local -> ext" || !isTrue(j.RemoveVanished) {
							return fmt.Errorf("unexpected job on server: %+v", j)
						}

						return nil
					}),
				),
			},
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "hourly"
  comment      = "local -> ext, hourly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "schedule", "hourly"),
					resource.TestCheckNoResourceAttr(syncJobResourceName, "remote"),
					resource.TestCheckResourceAttr(syncJobResourceName, "remove_vanished", "false"),
					resource.TestCheckResourceAttrSet(syncJobResourceName, "digest"),
					checkLocalSyncJobOnServer(te, id, src, dst, func(j *jobs.SyncJob) error {
						if j.Schedule != "hourly" || j.Comment != "local -> ext, hourly" || isTrue(j.RemoveVanished) {
							return fmt.Errorf("unexpected job on server: %+v", j)
						}

						return nil
					}),
				),
			},
			{
				ResourceName:                         syncJobResourceName,
				ImportState:                          true,
				ImportStateId:                        id,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
			},
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "hourly"
  comment      = "local -> ext, hourly"
}`),
				Check: func(_ *terraform.State) error {
					// Remove the job behind Terraform's back.
					return te.Client().Jobs.DeleteSyncJob(te.Context(), id)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: te.RenderConfig(syncLocalBaseConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "hourly"
  comment      = "local -> ext, hourly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionCreate),
					},
				},
				Check: checkLocalSyncJobOnServer(te, id, src, dst, nil),
			},
		},
	})
}

// PBS allows switching a sync job between local and remote in place;
// switching back to local must remove the remote on the server.
//
//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceSyncJobSwitchLocalRemote(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddSelfRemoteVars()

	src, dst := addLocalSyncDatastoreVars(te)
	id := te.RandomName("sync")
	remote := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"ID": id, "Remote": remote})

	const remoteConfig = `
resource "pbs_remote" "test" {
  name        = "{{.Remote}}"
  host        = "{{.RemoteHost}}"
  auth_id     = "{{.RemoteAuthID}}"
  password    = "{{.RemotePassword}}"
  fingerprint = "{{.RemoteFingerprint}}"
}
`

	localConfig := te.RenderConfig(syncLocalBaseConfig + remoteConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote_store = pbs_datastore.src.name
  schedule     = "daily"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkSyncJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: localConfig,
				Check:  checkLocalSyncJobOnServer(te, id, src, dst, nil),
			},
			{
				Config: te.RenderConfig(syncLocalBaseConfig + remoteConfig + `
resource "pbs_sync_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.dst.name
  remote       = pbs_remote.test.name
  remote_store = pbs_datastore.src.name
  schedule     = "daily"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(syncJobResourceName, "remote", remote),
					resource.TestCheckResourceAttrSet(syncJobResourceName, "digest"),
					checkSyncJobOnServer(te, id, func(j *jobs.SyncJob) error {
						if j.Remote != remote || j.RemoteStore != src || j.Store != dst {
							return fmt.Errorf("expected remote sync via %s, got %+v", remote, j)
						}

						return nil
					}),
				),
			},
			{
				Config: localConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(syncJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(syncJobResourceName, "remote"),
					checkLocalSyncJobOnServer(te, id, src, dst, nil),
				),
			},
		},
	})
}

// PBS rejects push sync jobs without a remote (with an unhelpful
// "permission check failed"), so the provider must reject them at plan time.
//
//testacc:tier=light
//testacc:resource=jobs
func TestAccResourceSyncJobPushRequiresRemote(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddTemplateVars(map[string]any{"ID": te.RandomName("sync")})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_sync_job" "test" {
  id             = "{{.ID}}"
  store          = "tfacc-local-src"
  remote_store   = "tfacc-local-dst"
  schedule       = "daily"
  sync_direction = "push"
}`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)push.*requires.*remote`),
			},
		},
	})
}
