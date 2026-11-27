/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package datastores_test

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
)

const resourceName = "pbs_datastore.test"

// digestRegexp matches the hex SHA-256 config digest PBS returns.
var digestRegexp = regexp.MustCompile(`^[0-9a-f]{64}$`)

func checkDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_datastore", "name", func(id string) error {
		_, err := te.Client().Datastores.GetDatastore(te.Context(), id)
		return err
	})
}

// checkDatastoreOnServer verifies the datastore configuration directly via the
// PBS API, independent of what the provider wrote to state.
func checkDatastoreOnServer(te *test.Environment, name, comment, gcSchedule string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ds, err := te.Client().Datastores.GetDatastore(te.Context(), name)
		if err != nil {
			return fmt.Errorf("reading datastore %q from API: %w", name, err)
		}

		if ds.Comment != comment {
			return fmt.Errorf("API comment = %q, want %q", ds.Comment, comment)
		}

		if ds.GCSchedule != gcSchedule {
			return fmt.Errorf("API gc-schedule = %q, want %q", ds.GCSchedule, gcSchedule)
		}

		return nil
	}
}

//testacc:tier=medium
//testacc:resource=datastore
func TestAccResourceDatastoreDirectory(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Name":    name,
		"Path":    te.DatastorePath(name),
		"NewPath": te.DatastorePath(name + "-moved"),
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_datastore" "test" {
  name        = "{{.Name}}"
  path        = "{{.Path}}"
  comment     = "created by acceptance test"
  gc_schedule = "daily"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "path", te.DatastorePath(name)),
					resource.TestCheckResourceAttr(resourceName, "comment", "created by acceptance test"),
					resource.TestCheckResourceAttr(resourceName, "gc_schedule", "daily"),
					resource.TestCheckResourceAttr(resourceName, "removable", "false"),
					checkDatastoreOnServer(te, name, "created by acceptance test", "daily"),
				),
			},
			{
				// Mutable fields must be updated in place, never by replacement.
				Config: te.RenderConfig(`
resource "pbs_datastore" "test" {
  name        = "{{.Name}}"
  path        = "{{.Path}}"
  comment     = "updated by acceptance test"
  gc_schedule = "weekly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "comment", "updated by acceptance test"),
					resource.TestCheckResourceAttr(resourceName, "gc_schedule", "weekly"),
					checkDatastoreOnServer(te, name, "updated by acceptance test", "weekly"),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// digest hashes the whole config file, so parallel tests touching other
				// objects of the same type change it between apply and import.
				ImportStateVerifyIgnore: []string{"digest"},
			},
			{
				// Changing the path is not supported by PBS and must force replacement.
				Config: te.RenderConfig(`
resource "pbs_datastore" "test" {
  name        = "{{.Name}}"
  path        = "{{.NewPath}}"
  comment     = "updated by acceptance test"
  gc_schedule = "weekly"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.TestCheckResourceAttr(resourceName, "path", te.DatastorePath(name+"-moved")),
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=datastore
func TestAccResourceDatastoreDigest(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("ds")
	other := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Name":      name,
		"Path":      te.DatastorePath(name),
		"Other":     other,
		"OtherPath": te.DatastorePath(other),
	})

	// Two datastores in the same datastore.cfg, both changed in one apply: the
	// digest covers the whole file, so it must not be used for optimistic locking.
	config := `
resource "pbs_datastore" "test" {
  name    = "{{.Name}}"
  path    = "{{.Path}}"
  comment = "{{.Comment}}"
}

resource "pbs_datastore" "other" {
  name    = "{{.Other}}"
  path    = "{{.OtherPath}}"
  comment = "{{.Comment}}"
}`
	render := func(comment string) string {
		te.AddTemplateVars(map[string]any{"Comment": comment})
		return te.RenderConfig(config)
	}

	digestChanges := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: render("first"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_datastore.other", "digest", digestRegexp),
					func(_ *terraform.State) error {
						ds, err := te.Client().Datastores.GetDatastore(te.Context(), name)
						if err != nil {
							return fmt.Errorf("reading datastore %q from API: %w", name, err)
						}

						if !digestRegexp.MatchString(ds.Digest) {
							return fmt.Errorf("API digest = %q, want 64 hex chars", ds.Digest)
						}

						return nil
					},
				),
				ConfigStateChecks: []statecheck.StateCheck{
					digestChanges.AddStateValue(resourceName, tfjsonpath.New("digest")),
				},
			},
			{
				Config: render("second"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("pbs_datastore.other", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(resourceName, tfjsonpath.New("digest")),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_datastore.other", "digest", digestRegexp),
					func(_ *terraform.State) error {
						for _, n := range []string{name, other} {
							ds, err := te.Client().Datastores.GetDatastore(te.Context(), n)
							if err != nil {
								return fmt.Errorf("reading datastore %q from API: %w", n, err)
							}

							if ds.Comment != "second" {
								return fmt.Errorf("datastore %q: API comment = %q, want %q", n, ds.Comment, "second")
							}
						}

						return nil
					},
				),
				ConfigStateChecks: []statecheck.StateCheck{
					// datastore.cfg was rewritten, so the digest must have moved on.
					digestChanges.AddStateValue(resourceName, tfjsonpath.New("digest")),
				},
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=datastore
func TestAccResourceDatastoreDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{"Name": name, "Path": te.DatastorePath(name)})

	config := te.RenderConfig(`
resource "pbs_datastore" "test" {
  name = "{{.Name}}"
  path = "{{.Path}}"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the datastore behind Terraform's back.
					return te.Client().Datastores.DeleteDatastoreWithOptions(te.Context(), name, true)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=datastore
func TestAccResourceDatastoreValidation(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "pbs_datastore" "test" {
  name = "tfacc-ds-missing-path"
}`,
				ExpectError: regexp.MustCompile(`(?i)path`),
			},
			{
				Config: `
resource "pbs_datastore" "test" {
  name      = "tfacc-ds-s3-no-bucket"
  path      = "/datastore/tfacc-ds-s3-no-bucket"
  s3_client = "endpoint-1"
}`,
				ExpectError: regexp.MustCompile(`(?i)s3_bucket|bucket`),
			},
			{
				Config: `
resource "pbs_datastore" "test" {
  name      = "tfacc-ds-removable"
  path      = "/datastore/tfacc-ds-removable"
  removable = true
}`,
				ExpectError: regexp.MustCompile(`(?i)backing_device`),
			},
		},
	})
}
