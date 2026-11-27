/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

const matchersConfig = `
resource "pbs_gotify_notification" "test" {
  name   = "{{.GotifyName}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token"
}

resource "pbs_notification_matcher" "full" {
  name           = "{{.FullName}}"
  targets        = [pbs_gotify_notification.test.name]
  match_severity = ["error", "warning"]
  match_field    = ["exact:type=prune"]
  match_calendar = ["mon..fri 8-17"]
  mode           = "any"
  invert_match   = true
  comment        = "full matcher"
  disable        = true
}

resource "pbs_notification_matcher" "minimal" {
  name = "{{.MinimalName}}"
}
`

func matcherNames(te *test.Environment) (full, minimal, gotify string) {
	full, minimal, gotify = te.RandomName("matcher"), te.RandomName("matcher"), te.RandomName("gotify")
	te.AddTemplateVars(map[string]any{"FullName": full, "MinimalName": minimal, "GotifyName": gotify})

	return full, minimal, gotify
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationMatcher(t *testing.T) {
	te := test.InitEnvironment(t)
	full, minimal, gotify := matcherNames(te)

	const dsFull = "data.pbs_notification_matcher.full"
	const dsMinimal = "data.pbs_notification_matcher.minimal"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(matchersConfig + `
data "pbs_notification_matcher" "full" {
  name = pbs_notification_matcher.full.name
}

data "pbs_notification_matcher" "minimal" {
  name = pbs_notification_matcher.minimal.name
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dsFull, "name", full),
					resource.TestCheckResourceAttr(dsFull, "targets.#", "1"),
					resource.TestCheckResourceAttr(dsFull, "targets.0", gotify),
					resource.TestCheckResourceAttr(dsFull, "match_severity.#", "2"),
					resource.TestCheckResourceAttr(dsFull, "match_field.0", "exact:type=prune"),
					resource.TestCheckResourceAttr(dsFull, "match_calendar.0", "mon..fri 8-17"),
					resource.TestCheckResourceAttr(dsFull, "mode", "any"),
					resource.TestCheckResourceAttr(dsFull, "invert_match", "true"),
					resource.TestCheckResourceAttr(dsFull, "comment", "full matcher"),
					resource.TestCheckResourceAttr(dsFull, "disable", "true"),

					// Defaults must be reported, not left null.
					resource.TestCheckResourceAttr(dsMinimal, "name", minimal),
					resource.TestCheckResourceAttr(dsMinimal, "mode", "all"),
					resource.TestCheckResourceAttr(dsMinimal, "invert_match", "false"),
					resource.TestCheckResourceAttr(dsMinimal, "disable", "false"),
					resource.TestCheckNoResourceAttr(dsMinimal, "targets.#"),
					resource.TestCheckNoResourceAttr(dsMinimal, "comment"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationMatchers(t *testing.T) {
	te := test.InitEnvironment(t)
	full, minimal, gotify := matcherNames(te)

	const ds = "data.pbs_notification_matchers.all"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(matchersConfig + `
data "pbs_notification_matchers" "all" {
  depends_on = [pbs_notification_matcher.full, pbs_notification_matcher.minimal]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs(ds, "matchers.*", map[string]string{
						"name":             full,
						"targets.#":        "1",
						"targets.0":        gotify,
						"match_severity.#": "2",
						"match_calendar.0": "mon..fri 8-17",
						"mode":             "any",
						"invert_match":     "true",
						"comment":          "full matcher",
						"disable":          "true",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "matchers.*", map[string]string{
						"name":         minimal,
						"mode":         "all",
						"invert_match": "false",
						"disable":      "false",
					}),
				),
			},
		},
	})
}

// TestAccDataSourceNotificationMatcherServerDefaults reads a matcher created
// outside Terraform with only a name, so PBS stores no mode/invert/disable.
//
//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationMatcherServerDefaults(t *testing.T) {
	te := test.InitEnvironment(t)
	name := te.RandomName("matcher")
	te.AddTemplateVars(map[string]any{"Name": name})

	const ds = "data.pbs_notification_matcher.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					err := te.Client().Notifications.CreateNotificationMatcher(te.Context(), &notifications.NotificationMatcher{Name: name})
					if err != nil {
						t.Fatalf("creating matcher: %v", err)
					}

					t.Cleanup(func() {
						_ = te.Client().Notifications.DeleteNotificationMatcher(context.Background(), name)
					})
				},
				Config: te.RenderConfig(`
data "pbs_notification_matcher" "test" {
  name = "{{.Name}}"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "mode", "all"),
					resource.TestCheckResourceAttr(ds, "invert_match", "false"),
					resource.TestCheckResourceAttr(ds, "disable", "false"),
				),
			},
		},
	})
}
