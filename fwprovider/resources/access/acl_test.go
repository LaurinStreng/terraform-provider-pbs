/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

const aclResourceName = "pbs_acl.test"

func aclCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_acl", "id", func(id string) error {
		parts := strings.Split(id, ":")
		if len(parts) != 3 {
			return fmt.Errorf("malformed pbs_acl id %q", id)
		}

		_, err := te.Client().Access.GetACL(te.Context(), parts[0], parts[1], parts[2])

		return err
	})
}

// aclCheckOnServer verifies that the ACL entry exists in PBS with the given
// propagate flag.
func aclCheckOnServer(te *test.Environment, aclPath, authID, role string, propagate bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		e, err := te.Client().Access.GetACL(te.Context(), aclPath, authID, role)
		if err != nil {
			return fmt.Errorf("reading ACL %s on %s for %s from API: %w", role, aclPath, authID, err)
		}

		if e.Propagate != propagate {
			return fmt.Errorf("API propagate = %t, want %t", e.Propagate, propagate)
		}

		return nil
	}
}

// aclCheckGoneOnServer verifies that the ACL entry does not exist in PBS.
func aclCheckGoneOnServer(te *test.Environment, aclPath, authID, role string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := te.Client().Access.GetACL(te.Context(), aclPath, authID, role)
		if !test.IsNotFound(err) {
			return fmt.Errorf("ACL %s on %s for %s should be gone, got err=%v", role, aclPath, authID, err)
		}

		return nil
	}
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceACLUser(t *testing.T) {
	te := test.InitEnvironment(t)

	userID := te.RandomName("user") + "@pbs"
	// The datastore does not need to exist for an ACL on its path.
	aclPath := "/datastore/" + te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"UserID": userID,
		"Path":   aclPath,
	})

	render := func(role, propagate string) string {
		te.AddTemplateVars(map[string]any{"Role": role, "Propagate": propagate})

		return te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_acl" "test" {
  path    = "{{.Path}}"
  auth_id = pbs_user.test.userid
  role    = "{{.Role}}"
{{- if .Propagate}}
  propagate = {{.Propagate}}
{{- end}}
}`)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             aclCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				// propagate defaults to true.
				Config: render("DatastoreAudit", ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(aclResourceName, "id", aclPath+":"+userID+":DatastoreAudit"),
					resource.TestCheckResourceAttr(aclResourceName, "path", aclPath),
					resource.TestCheckResourceAttr(aclResourceName, "auth_id", userID),
					resource.TestCheckResourceAttr(aclResourceName, "role", "DatastoreAudit"),
					resource.TestCheckResourceAttr(aclResourceName, "propagate", "true"),
					aclCheckOnServer(te, aclPath, userID, "DatastoreAudit", true),
				),
			},
			{
				// propagate is updated in place.
				Config: render("DatastoreAudit", "false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aclResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(aclResourceName, "propagate", "false"),
					aclCheckOnServer(te, aclPath, userID, "DatastoreAudit", false),
				),
			},
			{
				ResourceName:      aclResourceName,
				ImportState:       true,
				ImportStateIdFunc: test.ImportStateIDFromAttr(aclResourceName, "id"),
				ImportStateVerify: true,
			},
			{
				// The role is part of the entry's identity and forces replacement.
				Config: render("DatastoreReader", "false"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aclResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(aclResourceName, "id", aclPath+":"+userID+":DatastoreReader"),
					aclCheckOnServer(te, aclPath, userID, "DatastoreReader", false),
					aclCheckGoneOnServer(te, aclPath, userID, "DatastoreAudit"),
				),
			},
			{
				// Removing propagate from the config resets it to the default.
				Config: render("DatastoreReader", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aclResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(aclResourceName, "propagate", "true"),
					aclCheckOnServer(te, aclPath, userID, "DatastoreReader", true),
				),
			},
			{
				// Remove the entry behind Terraform's back.
				PreConfig: func() {
					if err := te.Client().Access.DeleteACL(te.Context(), aclPath, userID, "DatastoreReader"); err != nil {
						t.Fatalf("deleting ACL out of band: %v", err)
					}
				},
				// Refresh must drop the resource from state and plan a re-create.
				Config: render("DatastoreReader", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aclResourceName, plancheck.ResourceActionCreate),
					},
				},
				Check: aclCheckOnServer(te, aclPath, userID, "DatastoreReader", true),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceACLAPIToken(t *testing.T) {
	te := test.InitEnvironment(t)

	// "tfacc-user-xxxxxxxx@pbs!tfacc-token-xxxxxxxx" is 44 characters, well
	// below PBS's 64-character limit for token IDs.
	userID := te.RandomName("user") + "@pbs"
	tokenName := te.RandomName("token")
	tokenID := userID + "!" + tokenName
	te.AddTemplateVars(map[string]any{"UserID": userID, "TokenName": tokenName})

	config := te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
}

resource "pbs_acl" "test" {
  path    = "/system"
  auth_id = pbs_api_token.test.tokenid
  role    = "Audit"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             aclCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(aclResourceName, "auth_id", tokenID),
					resource.TestCheckResourceAttr(aclResourceName, "id", "/system:"+tokenID+":Audit"),
					resource.TestCheckResourceAttr(aclResourceName, "propagate", "true"),
					aclCheckOnServer(te, "/system", tokenID, "Audit", true),
				),
			},
			{
				ResourceName:      aclResourceName,
				ImportState:       true,
				ImportStateId:     "/system:" + tokenID + ":Audit",
				ImportStateVerify: true,
			},
			{
				// Deleting the user removes its tokens and their ACL entries in
				// PBS; refresh must notice that and re-create all of them.
				PreConfig: func() {
					if err := te.Client().Access.DeleteUser(te.Context(), userID); err != nil {
						t.Fatalf("deleting user out of band: %v", err)
					}
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(aclResourceName, plancheck.ResourceActionCreate),
					},
				},
				Check: aclCheckOnServer(te, "/system", tokenID, "Audit", true),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceACLValidation(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "pbs_acl" "test" {
  path    = "/system"
  auth_id = "root@pam"
  role    = "SuperUser"
}`,
				ExpectError: regexp.MustCompile(`(?i)value\s+must\s+be\s+one\s+of`),
			},
			{
				Config: `
resource "pbs_acl" "test" {
  path    = "datastore/store1"
  auth_id = "root@pam"
  role    = "Audit"
}`,
				ExpectError: regexp.MustCompile(`(?i)absolute\s+path`),
			},
			{
				Config: `
resource "pbs_acl" "test" {
  path    = "/system"
  auth_id = "root"
  role    = "Audit"
}`,
				ExpectError: regexp.MustCompile(`user@realm`),
			},
			{
				Config: `
resource "pbs_acl" "test" {
  path    = "/system"
  auth_id = "root@pam"
  role    = "Audit"
}`,
				ResourceName:  aclResourceName,
				ImportState:   true,
				ImportStateId: "/system:root@pam",
				ExpectError:   regexp.MustCompile(`(?i)invalid\s+import\s+id`),
			},
		},
	})
}
