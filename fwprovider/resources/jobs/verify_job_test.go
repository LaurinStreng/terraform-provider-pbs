/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package jobs_test

import (
	"fmt"
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

const verifyJobResourceName = "pbs_verify_job.test"

func checkVerifyJobDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_verify_job", "id", func(id string) error {
		_, err := te.Client().Jobs.GetVerifyJob(te.Context(), id)
		return err
	})
}

func checkVerifyJobOnServer(te *test.Environment, id string, fn func(*jobs.VerifyJob) error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		job, err := te.Client().Jobs.GetVerifyJob(te.Context(), id)
		if err != nil {
			return fmt.Errorf("reading verify job %q from API: %w", id, err)
		}

		return fn(job)
	}
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceVerifyJob(t *testing.T) {
	te := test.InitEnvironment(t)

	store := addDatastoreVars(te)
	id := te.RandomName("verify")
	newID := te.RandomName("verify")
	te.AddTemplateVars(map[string]any{"ID": id, "NewID": newID})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkVerifyJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id              = "{{.ID}}"
  store           = pbs_datastore.test.name
  schedule        = "daily"
  ignore_verified = true
  outdated_after  = 30
  max_depth       = 2
  comment         = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(verifyJobResourceName, "id", id),
					resource.TestCheckResourceAttr(verifyJobResourceName, "store", store),
					resource.TestCheckResourceAttr(verifyJobResourceName, "schedule", "daily"),
					resource.TestCheckResourceAttr(verifyJobResourceName, "ignore_verified", "true"),
					resource.TestCheckResourceAttr(verifyJobResourceName, "outdated_after", "30"),
					resource.TestCheckResourceAttr(verifyJobResourceName, "max_depth", "2"),
					resource.TestCheckNoResourceAttr(verifyJobResourceName, "namespace"),
					checkVerifyJobOnServer(te, id, func(j *jobs.VerifyJob) error {
						if j.Store != store || j.Schedule != "daily" || j.Comment != "created by acceptance test" {
							return fmt.Errorf("unexpected job on server: store=%q schedule=%q comment=%q", j.Store, j.Schedule, j.Comment)
						}

						if j.IgnoreVerified == nil || !*j.IgnoreVerified {
							return fmt.Errorf("ignore-verified not set on server")
						}

						if intPtrString(j.OutdatedAfter) != "30" || intPtrString(j.MaxDepth) != "2" {
							return fmt.Errorf("outdated-after=%s max-depth=%s, want 30/2",
								intPtrString(j.OutdatedAfter), intPtrString(j.MaxDepth))
						}

						return nil
					}),
				),
			},
			{
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id              = "{{.ID}}"
  store           = pbs_datastore.test.name
  schedule        = "weekly"
  ignore_verified = false
  outdated_after  = 60
  max_depth       = 4
  namespace       = "tfacc-ns"
  comment         = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(verifyJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(verifyJobResourceName, "schedule", "weekly"),
					resource.TestCheckResourceAttr(verifyJobResourceName, "ignore_verified", "false"),
					resource.TestCheckResourceAttr(verifyJobResourceName, "namespace", "tfacc-ns"),
					checkVerifyJobOnServer(te, id, func(j *jobs.VerifyJob) error {
						if j.Schedule != "weekly" || j.Comment != "updated by acceptance test" || j.Namespace != "tfacc-ns" {
							return fmt.Errorf("unexpected job on server: schedule=%q comment=%q ns=%q", j.Schedule, j.Comment, j.Namespace)
						}

						if j.IgnoreVerified != nil && *j.IgnoreVerified {
							return fmt.Errorf("ignore-verified still true on server")
						}

						if intPtrString(j.OutdatedAfter) != "60" || intPtrString(j.MaxDepth) != "4" {
							return fmt.Errorf("outdated-after=%s max-depth=%s, want 60/4",
								intPtrString(j.OutdatedAfter), intPtrString(j.MaxDepth))
						}

						return nil
					}),
				),
			},
			{
				ResourceName:                         verifyJobResourceName,
				ImportState:                          true,
				ImportStateId:                        id,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "id",
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id       = "{{.ID}}"
  store    = pbs_datastore.test.name
  schedule = "weekly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(verifyJobResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(verifyJobResourceName, "outdated_after"),
					resource.TestCheckNoResourceAttr(verifyJobResourceName, "max_depth"),
					resource.TestCheckNoResourceAttr(verifyJobResourceName, "namespace"),
					resource.TestCheckNoResourceAttr(verifyJobResourceName, "comment"),
					checkVerifyJobOnServer(te, id, func(j *jobs.VerifyJob) error {
						if j.OutdatedAfter != nil || j.MaxDepth != nil || j.Namespace != "" || j.Comment != "" {
							return fmt.Errorf("optional fields still set on server: %+v", j)
						}

						return nil
					}),
				),
			},
			{
				// The job ID is immutable and forces replacement.
				Config: te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id       = "{{.NewID}}"
  store    = pbs_datastore.test.name
  schedule = "weekly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(verifyJobResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(verifyJobResourceName, "id", newID),
					checkVerifyJobOnServer(te, newID, func(*jobs.VerifyJob) error { return nil }),
					func(_ *terraform.State) error {
						_, err := te.Client().Jobs.GetVerifyJob(te.Context(), id)
						if !test.IsNotFound(err) {
							return fmt.Errorf("old verify job %q should be gone, got err=%v", id, err)
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
func TestAccResourceVerifyJobDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	addDatastoreVars(te)
	id := te.RandomName("verify")
	te.AddTemplateVars(map[string]any{"ID": id})

	config := te.RenderConfig(datastoreConfig + `
resource "pbs_verify_job" "test" {
  id       = "{{.ID}}"
  store    = pbs_datastore.test.name
  schedule = "daily"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkVerifyJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the job behind Terraform's back.
					return te.Client().Jobs.DeleteVerifyJob(te.Context(), id)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(verifyJobResourceName, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=jobs
func TestAccResourceVerifyJobDigest(t *testing.T) {
	te := test.InitEnvironment(t)

	addDatastoreVars(te)
	id := te.RandomName("verify")
	te.AddTemplateVars(map[string]any{"ID": id, "OtherID": te.RandomName("verify")})

	// Two jobs in the same verify.cfg, both changed in one apply: the digest
	// covers the whole file, so it must not be used for optimistic locking.
	config := datastoreConfig + `
resource "pbs_verify_job" "test" {
  id       = "{{.ID}}"
  store    = pbs_datastore.test.name
  schedule = "{{.Schedule}}"
}

resource "pbs_verify_job" "other" {
  id       = "{{.OtherID}}"
  store    = pbs_datastore.test.name
  schedule = "{{.Schedule}}"
}`
	render := func(schedule string) string {
		te.AddTemplateVars(map[string]any{"Schedule": schedule})
		return te.RenderConfig(config)
	}

	digestChanges := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkVerifyJobDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: render("daily"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(verifyJobResourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_verify_job.other", "digest", digestRegexp),
					checkVerifyJobOnServer(te, id, func(j *jobs.VerifyJob) error {
						if !digestRegexp.MatchString(j.Digest) {
							return fmt.Errorf("API digest = %q, want 64 hex chars", j.Digest)
						}

						return nil
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					digestChanges.AddStateValue(verifyJobResourceName, tfjsonpath.New("digest")),
				},
			},
			{
				Config: render("weekly"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(verifyJobResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("pbs_verify_job.other", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(verifyJobResourceName, tfjsonpath.New("digest")),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(verifyJobResourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_verify_job.other", "digest", digestRegexp),
					checkVerifyJobOnServer(te, id, func(j *jobs.VerifyJob) error {
						if j.Schedule != "weekly" {
							return fmt.Errorf("API schedule = %q, want weekly", j.Schedule)
						}

						return nil
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					// verify.cfg was rewritten, so the digest must have moved on.
					digestChanges.AddStateValue(verifyJobResourceName, tfjsonpath.New("digest")),
				},
			},
		},
	})
}
