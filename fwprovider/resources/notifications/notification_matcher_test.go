/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

const matcherResource = "pbs_notification_matcher.test"

func matcherCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_notification_matcher", "name", func(id string) error {
		_, err := te.Client().Notifications.GetNotificationMatcher(te.Context(), id)
		return err
	})
}

func checkMatcherOnServer(te *test.Environment, want notifications.NotificationMatcher) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := te.Client().Notifications.GetNotificationMatcher(te.Context(), want.Name)
		if err != nil {
			return err
		}

		mode := got.Mode
		if mode == "" {
			mode = "all" // PBS omits the default mode
		}

		return errors.Join(
			checkSlice("target", got.Targets, want.Targets),
			checkSlice("match-severity", got.MatchSeverity, want.MatchSeverity),
			checkSlice("match-field", got.MatchField, want.MatchField),
			checkSlice("match-calendar", got.MatchCalendar, want.MatchCalendar),
			checkEqual("mode", mode, want.Mode),
			checkEqual("invert-match", boolOrFalse(got.InvertMatch), boolOrFalse(want.InvertMatch)),
			checkEqual("comment", got.Comment, want.Comment),
			checkEqual("disable", boolOrFalse(got.Disable), boolOrFalse(want.Disable)),
		)
	}
}

// matcherTargets declares two endpoints the matcher can route to.
const matcherTargets = `
resource "pbs_gotify_notification" "a" {
  name   = "{{.GotifyName}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token"
}

resource "pbs_sendmail_notification" "b" {
  name         = "{{.SendmailName}}"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
}
`

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceNotificationMatcher(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("matcher")
	newName := te.RandomName("matcher")
	gotifyName := te.RandomName("gotify")
	sendmailName := te.RandomName("sendmail")
	te.AddTemplateVars(map[string]any{
		"Name":         name,
		"NewName":      newName,
		"GotifyName":   gotifyName,
		"SendmailName": sendmailName,
	})

	yes := true

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             matcherCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(matcherTargets + `
resource "pbs_notification_matcher" "test" {
  name           = "{{.Name}}"
  targets        = [pbs_gotify_notification.a.name]
  match_severity = ["error", "warning"]
  match_field    = ["exact:type=prune"]
  comment        = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(matcherResource, "name", name),
					resource.TestCheckResourceAttr(matcherResource, "targets.#", "1"),
					resource.TestCheckResourceAttr(matcherResource, "targets.0", gotifyName),
					resource.TestCheckResourceAttr(matcherResource, "match_severity.#", "2"),
					resource.TestCheckResourceAttr(matcherResource, "mode", "all"),
					resource.TestCheckResourceAttr(matcherResource, "invert_match", "false"),
					resource.TestCheckResourceAttr(matcherResource, "disable", "false"),
					checkMatcherOnServer(te, notifications.NotificationMatcher{
						Name:          name,
						Targets:       []string{gotifyName},
						MatchSeverity: []string{"error", "warning"},
						MatchField:    []string{"exact:type=prune"},
						Mode:          "all",
						Comment:       "created by acceptance test",
					}),
				),
			},
			{
				Config: te.RenderConfig(matcherTargets + `
resource "pbs_notification_matcher" "test" {
  name           = "{{.Name}}"
  targets        = [pbs_gotify_notification.a.name, pbs_sendmail_notification.b.name]
  match_severity = ["error"]
  match_field    = ["exact:type=gc"]
  match_calendar = ["mon..fri 8-17"]
  mode           = "any"
  invert_match   = true
  comment        = "updated by acceptance test"
  disable        = true
}`),
				ConfigPlanChecks: expectAction(matcherResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(matcherResource, "targets.#", "2"),
					resource.TestCheckResourceAttr(matcherResource, "mode", "any"),
					resource.TestCheckResourceAttr(matcherResource, "invert_match", "true"),
					resource.TestCheckResourceAttr(matcherResource, "disable", "true"),
					checkMatcherOnServer(te, notifications.NotificationMatcher{
						Name:          name,
						Targets:       []string{gotifyName, sendmailName},
						MatchSeverity: []string{"error"},
						MatchField:    []string{"exact:type=gc"},
						MatchCalendar: []string{"mon..fri 8-17"},
						Mode:          "any",
						InvertMatch:   &yes,
						Comment:       "updated by acceptance test",
						Disable:       &yes,
					}),
				),
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(matcherTargets + `
resource "pbs_notification_matcher" "test" {
  name = "{{.Name}}"
}`),
				ConfigPlanChecks: expectAction(matcherResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(matcherResource, "targets.#"),
					resource.TestCheckNoResourceAttr(matcherResource, "match_severity.#"),
					resource.TestCheckNoResourceAttr(matcherResource, "match_field.#"),
					resource.TestCheckNoResourceAttr(matcherResource, "match_calendar.#"),
					resource.TestCheckNoResourceAttr(matcherResource, "comment"),
					resource.TestCheckResourceAttr(matcherResource, "mode", "all"),
					resource.TestCheckResourceAttr(matcherResource, "invert_match", "false"),
					resource.TestCheckResourceAttr(matcherResource, "disable", "false"),
					checkMatcherOnServer(te, notifications.NotificationMatcher{
						Name: name,
						Mode: "all",
					}),
				),
			},
			{
				Config: te.RenderConfig(matcherTargets + `
resource "pbs_notification_matcher" "test" {
  name           = "{{.Name}}"
  targets        = [pbs_sendmail_notification.b.name]
  match_severity = ["info", "notice", "warning", "error"]
}`),
				ConfigPlanChecks: expectAction(matcherResource, plancheck.ResourceActionUpdate),
			},
			{
				ResourceName:                         matcherResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: te.RenderConfig(matcherTargets + `
resource "pbs_notification_matcher" "test" {
  name           = "{{.NewName}}"
  targets        = [pbs_sendmail_notification.b.name]
  match_severity = ["info", "notice", "warning", "error"]
}`),
				ConfigPlanChecks: expectAction(matcherResource, plancheck.ResourceActionReplace),
				Check: checkMatcherOnServer(te, notifications.NotificationMatcher{
					Name:          newName,
					Targets:       []string{sendmailName},
					MatchSeverity: []string{"info", "notice", "warning", "error"},
					Mode:          "all",
				}),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceNotificationMatcherDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("matcher")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_notification_matcher" "test" {
  name           = "{{.Name}}"
  match_severity = ["error"]
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             matcherCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.DeleteNotificationMatcher(te.Context(), name)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           config,
				ConfigPlanChecks: expectAction(matcherResource, plancheck.ResourceActionCreate),
			},
		},
	})
}
