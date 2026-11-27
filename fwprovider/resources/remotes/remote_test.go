/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package remotes_test

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

const (
	resourceName = "pbs_remote.test"
	// Remotes are not contacted on create/update, so syntactically valid
	// placeholder values are fine here.
	fingerprintA = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"
)

func checkDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_remote", "name", func(id string) error {
		_, err := te.Client().Remotes.GetRemote(te.Context(), id)
		return err
	})
}

type remoteWant struct {
	host        string
	port        int // 0 = unset
	authID      string
	fingerprint string
	comment     string
}

// checkRemoteOnServer verifies the remote configuration directly via the PBS API.
func checkRemoteOnServer(te *test.Environment, name string, want remoteWant) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		r, err := te.Client().Remotes.GetRemote(te.Context(), name)
		if err != nil {
			return fmt.Errorf("reading remote %q from API: %w", name, err)
		}

		if r.Host != want.host {
			return fmt.Errorf("API host = %q, want %q", r.Host, want.host)
		}

		if r.AuthID != want.authID {
			return fmt.Errorf("API auth-id = %q, want %q", r.AuthID, want.authID)
		}

		gotPort := 0
		if r.Port != nil {
			gotPort = *r.Port
		}

		if gotPort != want.port {
			return fmt.Errorf("API port = %d, want %d", gotPort, want.port)
		}

		if r.Fingerprint != want.fingerprint {
			return fmt.Errorf("API fingerprint = %q, want %q", r.Fingerprint, want.fingerprint)
		}

		if r.Comment != want.comment {
			return fmt.Errorf("API comment = %q, want %q", r.Comment, want.comment)
		}

		return nil
	}
}

//testacc:tier=light
//testacc:resource=remotes
func TestAccResourceRemote(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("remote")
	newName := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{
		"Name":        name,
		"NewName":     newName,
		"Fingerprint": fingerprintA,
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_remote" "test" {
  name     = "{{.Name}}"
  host     = "pbs.example.com"
  auth_id  = "sync@pbs!test-token"
  password = "initial-password"
  comment  = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "host", "pbs.example.com"),
					resource.TestCheckResourceAttr(resourceName, "auth_id", "sync@pbs!test-token"),
					resource.TestCheckResourceAttr(resourceName, "password", "initial-password"),
					resource.TestCheckResourceAttr(resourceName, "comment", "created by acceptance test"),
					resource.TestCheckNoResourceAttr(resourceName, "port"),
					resource.TestCheckNoResourceAttr(resourceName, "fingerprint"),
					checkRemoteOnServer(te, name, remoteWant{
						host:    "pbs.example.com",
						authID:  "sync@pbs!test-token",
						comment: "created by acceptance test",
					}),
				),
			},
			{
				// All mutable fields, including the password, are updated in place.
				Config: te.RenderConfig(`
resource "pbs_remote" "test" {
  name        = "{{.Name}}"
  host        = "backup.example.com"
  port        = 8008
  auth_id     = "admin@pbs"
  password    = "new-password-54321"
  fingerprint = "{{.Fingerprint}}"
  comment     = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "host", "backup.example.com"),
					resource.TestCheckResourceAttr(resourceName, "port", "8008"),
					resource.TestCheckResourceAttr(resourceName, "auth_id", "admin@pbs"),
					resource.TestCheckResourceAttr(resourceName, "password", "new-password-54321"),
					resource.TestCheckResourceAttr(resourceName, "fingerprint", fingerprintA),
					resource.TestCheckResourceAttr(resourceName, "comment", "updated by acceptance test"),
					checkRemoteOnServer(te, name, remoteWant{
						host:        "backup.example.com",
						port:        8008,
						authID:      "admin@pbs",
						fingerprint: fingerprintA,
						comment:     "updated by acceptance test",
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// PBS never returns the password; it is write-only.
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(`
resource "pbs_remote" "test" {
  name     = "{{.Name}}"
  host     = "backup.example.com"
  auth_id  = "admin@pbs"
  password = "new-password-54321"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "port"),
					resource.TestCheckNoResourceAttr(resourceName, "fingerprint"),
					resource.TestCheckNoResourceAttr(resourceName, "comment"),
					checkRemoteOnServer(te, name, remoteWant{
						host:   "backup.example.com",
						authID: "admin@pbs",
					}),
				),
			},
			{
				// The name is the remote's identity and forces replacement.
				Config: te.RenderConfig(`
resource "pbs_remote" "test" {
  name     = "{{.NewName}}"
  host     = "backup.example.com"
  auth_id  = "admin@pbs"
  password = "new-password-54321"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					checkRemoteOnServer(te, newName, remoteWant{
						host:   "backup.example.com",
						authID: "admin@pbs",
					}),
					func(_ *terraform.State) error {
						_, err := te.Client().Remotes.GetRemote(te.Context(), name)
						if !test.IsNotFound(err) {
							return fmt.Errorf("old remote %q should be gone, got err=%v", name, err)
						}

						return nil
					},
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=remotes
func TestAccResourceRemoteDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_remote" "test" {
  name     = "{{.Name}}"
  host     = "pbs.example.com"
  auth_id  = "admin@pbs"
  password = "secret"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the remote behind Terraform's back.
					return te.Client().Remotes.DeleteRemote(te.Context(), name)
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
//testacc:resource=remotes
func TestAccResourceRemoteDigest(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("remote")
	te.AddTemplateVars(map[string]any{"Name": name, "Other": te.RandomName("remote")})

	// Two remotes in the same remote.cfg, both changed in one apply: the digest
	// covers the whole file, so it must not be used for optimistic locking.
	config := `
resource "pbs_remote" "test" {
  name     = "{{.Name}}"
  host     = "pbs.example.com"
  auth_id  = "admin@pbs"
  password = "secret"
  comment  = "{{.Comment}}"
}

resource "pbs_remote" "other" {
  name     = "{{.Other}}"
  host     = "pbs.example.com"
  auth_id  = "admin@pbs"
  password = "secret"
  comment  = "{{.Comment}}"
}`
	render := func(comment string) string {
		te.AddTemplateVars(map[string]any{"Comment": comment})
		return te.RenderConfig(config)
	}

	digestRegexp := regexp.MustCompile(`^[0-9a-f]{64}$`)
	digestChanges := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: render("first"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_remote.other", "digest", digestRegexp),
					func(_ *terraform.State) error {
						r, err := te.Client().Remotes.GetRemote(te.Context(), name)
						if err != nil {
							return fmt.Errorf("reading remote %q from API: %w", name, err)
						}

						if !digestRegexp.MatchString(r.Digest) {
							return fmt.Errorf("API digest = %q, want 64 hex chars", r.Digest)
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
						plancheck.ExpectResourceAction("pbs_remote.other", plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(resourceName, tfjsonpath.New("digest")),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(resourceName, "digest", digestRegexp),
					resource.TestMatchResourceAttr("pbs_remote.other", "digest", digestRegexp),
					func(_ *terraform.State) error {
						r, err := te.Client().Remotes.GetRemote(te.Context(), name)
						if err != nil {
							return fmt.Errorf("reading remote %q from API: %w", name, err)
						}

						if r.Comment != "second" {
							return fmt.Errorf("API comment = %q, want %q", r.Comment, "second")
						}

						return nil
					},
				),
				ConfigStateChecks: []statecheck.StateCheck{
					// remote.cfg was rewritten, so the digest must have moved on.
					digestChanges.AddStateValue(resourceName, tfjsonpath.New("digest")),
				},
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=remotes
func TestAccResourceRemoteValidation(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "pbs_remote" "test" {
  name     = "ab"
  host     = "pbs.example.com"
  auth_id  = "admin@pbs"
  password = "secret"
}`,
				ExpectError: regexp.MustCompile(`(?i)length`),
			},
			{
				Config: `
resource "pbs_remote" "test" {
  name     = "tfacc-remote-invalid-auth"
  host     = "pbs.example.com"
  auth_id  = "invalid-format"
  password = "secret"
}`,
				ExpectError: regexp.MustCompile(`user@realm`),
			},
			{
				Config: `
resource "pbs_remote" "test" {
  name        = "tfacc-remote-invalid-fp"
  host        = "pbs.example.com"
  auth_id     = "admin@pbs"
  password    = "secret"
  fingerprint = "invalid-fingerprint"
}`,
				ExpectError: regexp.MustCompile(`(?i)hexadecimal`),
			},
			{
				Config: `
resource "pbs_remote" "test" {
  name     = "tfacc-remote-invalid-port"
  host     = "pbs.example.com"
  port     = 99999
  auth_id  = "admin@pbs"
  password = "secret"
}`,
				ExpectError: regexp.MustCompile(`(?i)between 1 and 65535`),
			},
		},
	})
}
