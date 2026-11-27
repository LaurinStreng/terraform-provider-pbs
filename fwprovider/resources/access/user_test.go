/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

const (
	userResourceName = "pbs_user.test"
	// 2100-01-01T00:00:00Z, far enough in the future to not lock the user out.
	userExpireEpoch = 4102444800
)

func userCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_user", "userid", func(id string) error {
		_, err := te.Client().Access.GetUser(te.Context(), id)
		return err
	})
}

type userWant struct {
	comment   string
	firstname string
	lastname  string
	email     string
	enable    bool
	expire    int64
}

// checkUserOnServer verifies the user directly via the PBS API, mapping
// omitted enable/expire to PBS's defaults (true / 0).
func checkUserOnServer(te *test.Environment, userid string, want userWant) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		u, err := te.Client().Access.GetUser(te.Context(), userid)
		if err != nil {
			return fmt.Errorf("reading user %q from API: %w", userid, err)
		}

		if u.Comment != want.comment {
			return fmt.Errorf("API comment = %q, want %q", u.Comment, want.comment)
		}

		if u.Firstname != want.firstname {
			return fmt.Errorf("API firstname = %q, want %q", u.Firstname, want.firstname)
		}

		if u.Lastname != want.lastname {
			return fmt.Errorf("API lastname = %q, want %q", u.Lastname, want.lastname)
		}

		if u.Email != want.email {
			return fmt.Errorf("API email = %q, want %q", u.Email, want.email)
		}

		gotEnable := u.Enable == nil || *u.Enable
		if gotEnable != want.enable {
			return fmt.Errorf("API enable = %t, want %t", gotEnable, want.enable)
		}

		var gotExpire int64
		if u.Expire != nil {
			gotExpire = *u.Expire
		}

		if gotExpire != want.expire {
			return fmt.Errorf("API expire = %d, want %d", gotExpire, want.expire)
		}

		return nil
	}
}

// checkUserPassword verifies on the server that the user can log in with
// password and is rejected with rejected.
func checkUserPassword(te *test.Environment, userid, password, rejected string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if err := te.CheckLogin(api.Credentials{Username: userid, Password: password}); err != nil {
			return fmt.Errorf("login of %q with expected password failed: %w", userid, err)
		}

		if err := te.CheckLogin(api.Credentials{Username: userid, Password: rejected}); err == nil {
			return fmt.Errorf("login of %q with password %q succeeded, want rejection", userid, rejected)
		}

		return nil
	}
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceUser(t *testing.T) {
	te := test.InitEnvironment(t)

	userid := te.RandomName("user") + "@pbs"
	newUserid := te.RandomName("user") + "@pbs"
	te.AddTemplateVars(map[string]any{
		"UserID":    userid,
		"NewUserID": newUserid,
		"Expire":    userExpireEpoch,
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             userCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid    = "{{.UserID}}"
  comment   = "created by acceptance test"
  firstname = "Alice"
  lastname  = "Example"
  email     = "alice@example.com"
  password  = "initial-password-1"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(userResourceName, "userid", userid),
					resource.TestCheckResourceAttr(userResourceName, "comment", "created by acceptance test"),
					resource.TestCheckResourceAttr(userResourceName, "firstname", "Alice"),
					resource.TestCheckResourceAttr(userResourceName, "lastname", "Example"),
					resource.TestCheckResourceAttr(userResourceName, "email", "alice@example.com"),
					resource.TestCheckResourceAttr(userResourceName, "enable", "true"),
					resource.TestCheckResourceAttr(userResourceName, "expire", "0"),
					resource.TestCheckResourceAttr(userResourceName, "password", "initial-password-1"),
					checkUserOnServer(te, userid, userWant{
						comment:   "created by acceptance test",
						firstname: "Alice",
						lastname:  "Example",
						email:     "alice@example.com",
						enable:    true,
					}),
					checkUserPassword(te, userid, "initial-password-1", "wrong-password-0"),
				),
			},
			{
				// Mutable fields, including enable and expire, are updated in place.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid    = "{{.UserID}}"
  comment   = "updated by acceptance test"
  firstname = "Alicia"
  lastname  = "Sample"
  email     = "alicia@example.com"
  enable    = false
  expire    = {{.Expire}}
  password  = "initial-password-1"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(userResourceName, "comment", "updated by acceptance test"),
					resource.TestCheckResourceAttr(userResourceName, "firstname", "Alicia"),
					resource.TestCheckResourceAttr(userResourceName, "lastname", "Sample"),
					resource.TestCheckResourceAttr(userResourceName, "email", "alicia@example.com"),
					resource.TestCheckResourceAttr(userResourceName, "enable", "false"),
					resource.TestCheckResourceAttr(userResourceName, "expire", fmt.Sprint(userExpireEpoch)),
					checkUserOnServer(te, userid, userWant{
						comment:   "updated by acceptance test",
						firstname: "Alicia",
						lastname:  "Sample",
						email:     "alicia@example.com",
						enable:    false,
						expire:    userExpireEpoch,
					}),
				),
			},
			{
				ResourceName:                         userResourceName,
				ImportState:                          true,
				ImportStateId:                        userid,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "userid",
				// PBS never returns the password; it is write-only.
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				// Removing optional attributes must clear them on the server and
				// reset enable/expire to their defaults.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid   = "{{.UserID}}"
  password = "initial-password-1"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(userResourceName, "comment"),
					resource.TestCheckNoResourceAttr(userResourceName, "firstname"),
					resource.TestCheckNoResourceAttr(userResourceName, "lastname"),
					resource.TestCheckNoResourceAttr(userResourceName, "email"),
					resource.TestCheckResourceAttr(userResourceName, "enable", "true"),
					resource.TestCheckResourceAttr(userResourceName, "expire", "0"),
					checkUserOnServer(te, userid, userWant{enable: true}),
				),
			},
			{
				// The userid is the user's identity and forces replacement.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid   = "{{.NewUserID}}"
  password = "initial-password-1"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(userResourceName, "userid", newUserid),
					checkUserOnServer(te, newUserid, userWant{enable: true}),
					func(_ *terraform.State) error {
						_, err := te.Client().Access.GetUser(te.Context(), userid)
						if !test.IsNotFound(err) {
							return fmt.Errorf("old user %q should be gone, got err=%v", userid, err)
						}

						return nil
					},
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceUserPasswordChange(t *testing.T) {
	if os.Getenv("PBS_API_TOKEN") != "" {
		t.Skip("PBS rejects password changes when authenticated with an API token")
	}

	te := test.InitEnvironment(t)

	userid := te.RandomName("user") + "@pbs"
	te.AddTemplateVars(map[string]any{"UserID": userid})

	config := `
resource "pbs_user" "test" {
  userid   = "{{.UserID}}"
  password = "{{.Password}}"
}`
	render := func(password string) string {
		te.AddTemplateVars(map[string]any{"Password": password})
		return te.RenderConfig(config)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             userCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: render("first-password-1"),
				Check: resource.ComposeTestCheckFunc(
					checkUserOnServer(te, userid, userWant{enable: true}),
					checkUserPassword(te, userid, "first-password-1", "second-password-2"),
				),
			},
			{
				// A new password is applied in place, not by replacing the user
				// (which would delete its API tokens).
				Config: render("second-password-2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(userResourceName, "password", "second-password-2"),
					checkUserOnServer(te, userid, userWant{enable: true}),
					// Fails if Update does not call the change-password endpoint.
					checkUserPassword(te, userid, "second-password-2", "first-password-1"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceUserDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	userid := te.RandomName("user") + "@pbs"
	te.AddTemplateVars(map[string]any{"UserID": userid})

	config := te.RenderConfig(`
resource "pbs_user" "test" {
  userid  = "{{.UserID}}"
  comment = "deleted out of band"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             userCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the user behind Terraform's back.
					return te.Client().Access.DeleteUser(te.Context(), userid)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(userResourceName, plancheck.ResourceActionCreate),
					},
				},
				Check: checkUserOnServer(te, userid, userWant{comment: "deleted out of band", enable: true}),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceUserValidation(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "pbs_user" "test" {
  userid = "tfacc-user-without-realm"
}`,
				ExpectError: regexp.MustCompile(`name@realm`),
			},
			{
				Config: `
resource "pbs_user" "test" {
  userid    = "tfacc-user-short-name@pbs"
  firstname = "A"
}`,
				ExpectError: regexp.MustCompile(`(?i)length`),
			},
			{
				Config: `
resource "pbs_user" "test" {
  userid   = "tfacc-user-short-password@pbs"
  password = "short"
}`,
				ExpectError: regexp.MustCompile(`(?i)length`),
			},
			{
				Config: `
resource "pbs_user" "test" {
  userid = "tfacc-user-negative-expire@pbs"
  expire = -1
}`,
				ExpectError: regexp.MustCompile(`(?i)at least 0`),
			},
			{
				// Passwords of other realms are not managed by PBS; for pam this
				// would change the Linux system password.
				Config: `
resource "pbs_user" "test" {
  userid   = "x@pam"
  password = "some-password-1"
}`,
				ExpectError: regexp.MustCompile(`pbs realm`),
			},
		},
	})
}
