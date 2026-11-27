/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/jobs"
)

const pruneJobResourceName = "pbs_prune_job.test"

// digestRegexp matches the hex SHA-256 config digest PBS returns.
var digestRegexp = regexp.MustCompile(`^[0-9a-f]{64}$`)

// datastoreConfig is the datastore every job in this package runs against.
const datastoreConfig = `
resource "pbs_datastore" "test" {
  name = "{{.Store}}"
  path = "{{.StorePath}}"
}
`

func addDatastoreVars(te *test.Environment) string {
	store := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Store":     store,
		"StorePath": te.DatastorePath(store),
	})

	return store
}

func checkPruneJobDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_prune_job", "id", func(id string) error {
		_, err := te.Client().Jobs.GetPruneJob(te.Context(), id)
		return err
	})
}

func intPtrString(p *int) string {
	if p == nil {
		return "<nil>"
	}

	return fmt.Sprint(*p)
}

// checkPruneJobOnServer reads the job from the API and lets fn assert on it.
func checkPruneJobOnServer(te *test.Environment, id string, fn func(*jobs.PruneJob) error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		job, err := te.Client().Jobs.GetPruneJob(te.Context(), id)
		if err != nil {
			return fmt.Errorf("reading prune job %q from API: %w", id, err)
		}

		return fn(job)
	}
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourcePruneJob(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	id := te.RandomName("prune")
	newID := te.RandomName("prune")
	te.AddTemplateVars(map[string]any{"ID": id, "NewID": newID})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkPruneJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.test.name
  schedule     = "daily"
  keep_last    = 7
  keep_daily   = 14
  keep_weekly  = 8
  keep_monthly = 12
  keep_yearly  = 3
  max_depth    = 3
  comment      = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(pruneJobResourceName, "id", id),
					resource.TestCheckResourceAttr(pruneJobResourceName, "store", store),
					resource.TestCheckResourceAttr(pruneJobResourceName, "schedule", "daily"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "keep_last", "7"),
					resource.TestCheckNoResourceAttr(pruneJobResourceName, "keep_hourly"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "disable", "false"),
					checkPruneJobOnServer(te, id, func(j *jobs.PruneJob) error {
						if j.Store != store || j.Schedule != "daily" || j.Comment != "created by acceptance test" {
							return fmt.Errorf("unexpected job on server: store=%q schedule=%q comment=%q", j.Store, j.Schedule, j.Comment)
						}

						if intPtrString(j.KeepLast) != "7" || intPtrString(j.KeepDaily) != "14" ||
							intPtrString(j.KeepWeekly) != "8" || intPtrString(j.KeepMonthly) != "12" ||
							intPtrString(j.KeepYearly) != "3" || intPtrString(j.MaxDepth) != "3" {
							return fmt.Errorf("unexpected retention on server: %+v", j)
						}

						if j.KeepHourly != nil {
							return fmt.Errorf("keep-hourly = %d, want unset", *j.KeepHourly)
						}

						return nil
					}),
				),
			},
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id           = "{{.ID}}"
  store        = pbs_datastore.test.name
  schedule     = "weekly"
  keep_last    = 10
  keep_hourly  = 24
  keep_daily   = 21
  keep_weekly  = 12
  keep_monthly = 18
  keep_yearly  = 5
  max_depth    = 5
  namespace    = "tfacc-ns"
  disable      = true
  comment      = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(pruneJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(pruneJobResourceName, "schedule", "weekly"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "keep_hourly", "24"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "disable", "true"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "namespace", "tfacc-ns"),
					checkPruneJobOnServer(te, id, func(j *jobs.PruneJob) error {
						if j.Schedule != "weekly" || j.Comment != "updated by acceptance test" || j.Namespace != "tfacc-ns" {
							return fmt.Errorf("unexpected job on server: schedule=%q comment=%q ns=%q", j.Schedule, j.Comment, j.Namespace)
						}

						if intPtrString(j.KeepLast) != "10" || intPtrString(j.KeepHourly) != "24" ||
							intPtrString(j.KeepDaily) != "21" || intPtrString(j.KeepYearly) != "5" ||
							intPtrString(j.MaxDepth) != "5" {
							return fmt.Errorf("unexpected retention on server: %+v", j)
						}

						if j.Disable == nil || !*j.Disable {
							return fmt.Errorf("disable not set on server")
						}

						return nil
					}),
				),
			},
			{
				ResourceName:                         pruneJobResourceName,
				ImportState:                          true,
				ImportStateId:                        id,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id        = "{{.ID}}"
  store     = pbs_datastore.test.name
  schedule  = "weekly"
  keep_last = 10
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(pruneJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(pruneJobResourceName, "keep_daily"),
					resource.TestCheckNoResourceAttr(pruneJobResourceName, "max_depth"),
					resource.TestCheckNoResourceAttr(pruneJobResourceName, "comment"),
					resource.TestCheckNoResourceAttr(pruneJobResourceName, "namespace"),
					resource.TestCheckResourceAttr(pruneJobResourceName, "disable", "false"),
					checkPruneJobOnServer(te, id, func(j *jobs.PruneJob) error {
						if j.KeepHourly != nil || j.KeepDaily != nil || j.KeepWeekly != nil ||
							j.KeepMonthly != nil || j.KeepYearly != nil || j.MaxDepth != nil {
							return fmt.Errorf("optional retention fields still set on server: %+v", j)
						}

						if j.Comment != "" || j.Namespace != "" {
							return fmt.Errorf("comment = %q, ns = %q, want both empty", j.Comment, j.Namespace)
						}

						if j.Disable != nil && *j.Disable {
							return fmt.Errorf("job still disabled on server")
						}

						return nil
					}),
				),
			},
			{
				// The job ID is immutable and forces replacement.
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id        = "{{.NewID}}"
  store     = pbs_datastore.test.name
  schedule  = "weekly"
  keep_last = 10
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(pruneJobResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(pruneJobResourceName, "id", newID),
					checkPruneJobOnServer(te, newID, func(*jobs.PruneJob) error { return nil }),
					func(_ *terraform.State) error {
						_, err := te.Client().Jobs.GetPruneJob(te.Context(), id)
						if !test.IsNotFound(err) {
							return fmt.Errorf("old prune job %q should be gone, got err=%v", id, err)
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
func TestAccResourcePruneJobDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	addDatastoreVars(te)
	id := te.RandomName("prune")
	te.AddTemplateVars(map[string]any{"ID": id})

	config := te.RenderConfig(datastoreConfig + `
resource "pbs_prune_job" "test" {
  id        = "{{.ID}}"
  store     = pbs_datastore.test.name
  schedule  = "daily"
  keep_last = 3
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkPruneJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the job behind Terraform's back.
					return te.Client().Jobs.DeletePruneJob(te.Context(), id)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(pruneJobResourceName, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourcePruneJobDigest(t *testing.T) {
	te := test.InitEnvironment(t)

	addDatastoreVars(te)
	id := te.RandomName("prune")
	te.AddTemplateVars(map[string]any{"ID": id, "OtherID": te.RandomName("prune")})

	// Two jobs in the same prune.cfg, both changed in one apply: the digest
	// covers the whole file, so it must not be used for optimistic locking.
	config := datastoreConfig + `
resource "pbs_prune_job" "test" {
  id        = "{{.ID}}"
  store     = pbs_datastore.test.name
  schedule  = "{{.Schedule}}"
  keep_last = 3
}

resource "pbs_prune_job" "other" {
  id        = "{{.OtherID}}"
  store     = pbs_datastore.test.name
  schedule  = "{{.Schedule}}"
  keep_last = 3
}`
	render := func(schedule string) string {
		te.AddTemplateVars(map[string]any{"Schedule": schedule})
		return te.RenderConfig(config)
	}

	digestChanges := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkPruneJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: render("daily"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(pruneJobResourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_prune_job.other", "digest", digestRegexp),
					checkPruneJobOnServer(te, id, func(j *jobs.PruneJob) error {
						if !digestRegexp.MatchString(j.Digest) {
							return fmt.Errorf("API digest = %q, want 64 hex chars", j.Digest)
						}

						return nil
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					digestChanges.AddStateValue(pruneJobResourceName, tfjsonpath.New("digest")),
				},
			},
			{
				Config: render("weekly"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(pruneJobResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("pbs_prune_job.other", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(pruneJobResourceName, tfjsonpath.New("digest")),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(pruneJobResourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_prune_job.other", "digest", digestRegexp),
					checkPruneJobOnServer(te, id, func(j *jobs.PruneJob) error {
						if j.Schedule != "weekly" {
							return fmt.Errorf("API schedule = %q, want weekly", j.Schedule)
						}

						return nil
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					// prune.cfg was rewritten, so the digest must have moved on.
					digestChanges.AddStateValue(pruneJobResourceName, tfjsonpath.New("digest")),
				},
			},
		},
	})
}
