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

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

const (
	apiTokenResourceName = "pbs_api_token.test"
	// 2100-01-01T00:00:00Z
	apiTokenExpire = 4102444800
)

// apiTokenSplit splits "user@realm!name" into its parts.
func apiTokenSplit(id string) (string, string) {
	i := strings.LastIndex(id, "!")
	if i < 0 {
		return id, ""
	}

	return id[:i], id[i+1:]
}

func apiTokenCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_api_token", "tokenid", func(id string) error {
		userid, name := apiTokenSplit(id)
		_, err := te.Client().Access.GetAPIToken(te.Context(), userid, name)

		return err
	})
}

type apiTokenWant struct {
	comment string
	enable  bool
	expire  int64
}

// apiTokenCheckOnServer verifies the token directly via the PBS API.
func apiTokenCheckOnServer(te *test.Environment, userid, name string, want apiTokenWant) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		tok, err := te.Client().Access.GetAPIToken(te.Context(), userid, name)
		if err != nil {
			return fmt.Errorf("reading API token %s!%s from API: %w", userid, name, err)
		}

		if tok.Comment != want.comment {
			return fmt.Errorf("API comment = %q, want %q", tok.Comment, want.comment)
		}

		gotEnable := tok.Enable == nil || *tok.Enable
		if gotEnable != want.enable {
			return fmt.Errorf("API enable = %t, want %t", gotEnable, want.enable)
		}

		var gotExpire int64
		if tok.Expire != nil {
			gotExpire = *tok.Expire
		}

		if gotExpire != want.expire {
			return fmt.Errorf("API expire = %d, want %d", gotExpire, want.expire)
		}

		return nil
	}
}

func apiTokenCheckGone(te *test.Environment, userid, name string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := te.Client().Access.GetAPIToken(te.Context(), userid, name)
		if !test.IsNotFound(err) {
			return fmt.Errorf("API token %s!%s should be gone, got err=%v", userid, name, err)
		}

		return nil
	}
}

// apiTokenCaptureSecret stores the secret currently in state in *secret, so
// later steps can verify it against the server.
func apiTokenCaptureSecret(secret *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[apiTokenResourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", apiTokenResourceName)
		}

		v := rs.Primary.Attributes["value"]
		if v == "" {
			return fmt.Errorf("%s has no value in state", apiTokenResourceName)
		}

		*secret = v

		return nil
	}
}

// apiTokenCheckLogin authenticates against PBS with tokenid and *secret. A
// new token has no ACLs, but GET /access/users works for any authenticated
// caller. With wantOK=false the secret must be rejected (HTTP 401).
func apiTokenCheckLogin(te *test.Environment, tokenid string, secret *string, wantOK bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *secret == "" {
			return fmt.Errorf("no secret captured for %s", tokenid)
		}

		err := te.CheckLogin(api.Credentials{APIToken: tokenid + ":" + *secret})

		switch {
		case wantOK && err != nil:
			return fmt.Errorf("authenticating with the secret of %s failed: %w", tokenid, err)
		case !wantOK && err == nil:
			return fmt.Errorf("the old secret of %s still authenticates", tokenid)
		case !wantOK && !strings.Contains(err.Error(), "status 401"):
			return fmt.Errorf("expected the old secret of %s to be rejected with 401, got: %w", tokenid, err)
		}

		return nil
	}
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceAPIToken(t *testing.T) {
	te := test.InitEnvironment(t)

	userid := te.RandomName("user") + "@pbs"
	name := te.RandomName("token")
	newName := te.RandomName("token")
	te.AddTemplateVars(map[string]any{
		"UserID":    userid,
		"TokenName": name,
		"NewName":   newName,
		"Expire":    apiTokenExpire,
	})

	tokenID := userid + "!" + name

	// The secret must survive attribute updates and adding/removing
	// regenerate_trigger, and change only when the trigger value changes.
	// ValuesSame compares all recorded values, so the secrets before and after
	// regeneration need separate comparers.
	valueSame := statecheck.CompareValue(compare.ValuesSame())
	valueSameAfterRegen := statecheck.CompareValue(compare.ValuesSame())
	valueDiffers := statecheck.CompareValue(compare.ValuesDiffer())
	secretRegexp := regexp.MustCompile(`^\S+$`)

	var firstSecret, regenSecret, replacedSecret string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             apiTokenCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
  comment    = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(apiTokenResourceName, "userid", userid),
					resource.TestCheckResourceAttr(apiTokenResourceName, "token_name", name),
					resource.TestCheckResourceAttr(apiTokenResourceName, "tokenid", userid+"!"+name),
					resource.TestCheckResourceAttr(apiTokenResourceName, "comment", "created by acceptance test"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "enable", "true"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "expire", "0"),
					resource.TestMatchResourceAttr(apiTokenResourceName, "value", secretRegexp),
					apiTokenCheckOnServer(te, userid, name, apiTokenWant{
						comment: "created by acceptance test",
						enable:  true,
					}),
					apiTokenCaptureSecret(&firstSecret),
					apiTokenCheckLogin(te, tokenID, &firstSecret, true),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueSame.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				// comment, enable and expire are updated in place; the secret is kept.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
  comment    = "updated by acceptance test"
  enable     = false
  expire     = {{.Expire}}
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(apiTokenResourceName, tfjsonpath.New("tokenid"),
							knownvalue.StringExact(userid+"!"+name)),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(apiTokenResourceName, "comment", "updated by acceptance test"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "enable", "false"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "expire", fmt.Sprint(apiTokenExpire)),
					apiTokenCheckOnServer(te, userid, name, apiTokenWant{
						comment: "updated by acceptance test",
						enable:  false,
						expire:  apiTokenExpire,
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueSame.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				ResourceName:                         apiTokenResourceName,
				ImportState:                          true,
				ImportStateId:                        userid + "!" + name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "tokenid",
				// PBS never returns the token secret after creation, and
				// regenerate_trigger is a Terraform-only attribute.
				ImportStateVerifyIgnore: []string{"value", "regenerate_trigger"},
			},
			{
				// Removing optional attributes clears the comment and resets
				// enable/expire to their defaults on the server.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(apiTokenResourceName, "comment"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "enable", "true"),
					resource.TestCheckResourceAttr(apiTokenResourceName, "expire", "0"),
					apiTokenCheckOnServer(te, userid, name, apiTokenWant{enable: true}),
					apiTokenCheckLogin(te, tokenID, &firstSecret, true),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueSame.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				// Adding regenerate_trigger to an existing token (as after an
				// import) only records the value; the secret is kept.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid             = pbs_user.test.userid
  token_name         = "{{.TokenName}}"
  regenerate_trigger = "1"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(apiTokenResourceName, "regenerate_trigger", "1"),
					apiTokenCheckLogin(te, tokenID, &firstSecret, true),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueSame.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
					valueDiffers.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				// Changing regenerate_trigger issues a new secret in place;
				// the old secret is rejected by PBS afterwards.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid             = pbs_user.test.userid
  token_name         = "{{.TokenName}}"
  regenerate_trigger = "2"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(apiTokenResourceName, tfjsonpath.New("value")),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(apiTokenResourceName, "regenerate_trigger", "2"),
					resource.TestMatchResourceAttr(apiTokenResourceName, "value", secretRegexp),
					apiTokenCheckOnServer(te, userid, name, apiTokenWant{enable: true}),
					apiTokenCaptureSecret(&regenSecret),
					apiTokenCheckLogin(te, tokenID, &regenSecret, true),
					apiTokenCheckLogin(te, tokenID, &firstSecret, false),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueDiffers.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
					valueSameAfterRegen.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				// Removing regenerate_trigger keeps the current secret.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(apiTokenResourceName, "regenerate_trigger"),
					apiTokenCheckLogin(te, tokenID, &regenSecret, true),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueSameAfterRegen.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
			{
				// The token name is part of the token's identity and forces replacement.
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.NewName}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(apiTokenResourceName, "tokenid", userid+"!"+newName),
					resource.TestMatchResourceAttr(apiTokenResourceName, "value", secretRegexp),
					apiTokenCheckOnServer(te, userid, newName, apiTokenWant{enable: true}),
					apiTokenCheckGone(te, userid, name),
					apiTokenCaptureSecret(&replacedSecret),
					apiTokenCheckLogin(te, userid+"!"+newName, &replacedSecret, true),
					apiTokenCheckLogin(te, tokenID, &regenSecret, false),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					valueDiffers.AddStateValue(apiTokenResourceName, tfjsonpath.New("value")),
				},
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceAPITokenDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	userid := te.RandomName("user") + "@pbs"
	name := te.RandomName("token")
	te.AddTemplateVars(map[string]any{"UserID": userid, "TokenName": name})

	config := te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.UserID}}"
}

resource "pbs_api_token" "test" {
  userid     = pbs_user.test.userid
  token_name = "{{.TokenName}}"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             apiTokenCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the token behind Terraform's back.
					return te.Client().Access.DeleteAPIToken(te.Context(), userid, name)
				},
				// Refresh must drop the token from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(apiTokenResourceName, plancheck.ResourceActionCreate),
					},
				},
				Check: apiTokenCheckOnServer(te, userid, name, apiTokenWant{enable: true}),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccResourceAPITokenValidation(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "pbs_api_token" "test" {
  userid     = "tfacc-user-invalid@pbs"
  token_name = "-bad-name"
}`,
				ExpectError: regexp.MustCompile(`must\s+start\s+with\s+a\s+letter`),
			},
			{
				Config: `
resource "pbs_api_token" "test" {
  userid     = "tfacc-user-invalid@pbs"
  token_name = "ab"
}`,
				ExpectError: regexp.MustCompile(`(?i)length`),
			},
			{
				Config: `
resource "pbs_api_token" "test" {
  userid     = "tfacc-user-invalid"
  token_name = "tfacc-token-invalid"
}`,
				ExpectError: regexp.MustCompile(`user@realm`),
			},
			{
				Config: `
resource "pbs_api_token" "test" {
  userid     = "tfacc-user-invalid@pbs"
  token_name = "tfacc-token-invalid"
  comment    = "line one\nline two"
}`,
				ExpectError: regexp.MustCompile(`single\s+line`),
			},
			{
				Config: fmt.Sprintf(`
resource "pbs_api_token" "test" {
  userid     = "tfacc-user-%s@pbs"
  token_name = "tfacc-token-%s"
}`, strings.Repeat("a", 30), strings.Repeat("b", 20)),
				ExpectError: regexp.MustCompile(`at\s+most\s+64`),
			},
		},
	})
}
