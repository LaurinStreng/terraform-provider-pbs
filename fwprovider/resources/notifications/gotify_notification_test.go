/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

const gotifyResource = "pbs_gotify_notification.test"

func gotifyCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_gotify_notification", "name", func(id string) error {
		_, err := te.Client().Notifications.GetGotifyTarget(te.Context(), id)
		return err
	})
}

func checkGotifyOnServer(te *test.Environment, name, server, comment string, disable bool) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		g, err := te.Client().Notifications.GetGotifyTarget(te.Context(), name)
		if err != nil {
			return err
		}

		return errors.Join(
			checkEqual("server", g.Server, server),
			checkEqual("comment", g.Comment, comment),
			checkEqual("disable", boolOrFalse(g.Disable), disable),
		)
	}
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceGotifyNotification(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("gotify")
	newName := te.RandomName("gotify")
	server := te.RenderConfig("http://{{.GotifyHost}}:{{.GotifyPort}}")
	te.AddTemplateVars(map[string]any{"Name": name, "NewName": newName})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             gotifyCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name    = "{{.Name}}"
  server  = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token   = "tfacc-token-1"
  comment = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(gotifyResource, "name", name),
					resource.TestCheckResourceAttr(gotifyResource, "server", server),
					resource.TestCheckResourceAttr(gotifyResource, "comment", "created by acceptance test"),
					resource.TestCheckResourceAttr(gotifyResource, "disable", "false"),
					checkGotifyOnServer(te, name, server, "created by acceptance test", false),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name    = "{{.Name}}"
  server  = "http://{{.GotifyHost}}:{{.GotifyPort}}/updated"
  token   = "tfacc-token-2"
  comment = "updated by acceptance test"
  disable = true
}`),
				ConfigPlanChecks: expectAction(gotifyResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(gotifyResource, "server", server+"/updated"),
					resource.TestCheckResourceAttr(gotifyResource, "disable", "true"),
					checkGotifyOnServer(te, name, server+"/updated", "updated by acceptance test", true),
				),
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name   = "{{.Name}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token-2"
}`),
				ConfigPlanChecks: expectAction(gotifyResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(gotifyResource, "comment"),
					resource.TestCheckResourceAttr(gotifyResource, "disable", "false"),
					checkGotifyOnServer(te, name, server, "", false),
				),
			},
			{
				ResourceName:                         gotifyResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// PBS never returns the token.
				ImportStateVerifyIgnore: []string{"token"},
			},
			{
				Config: te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name   = "{{.NewName}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token-2"
}`),
				ConfigPlanChecks: expectAction(gotifyResource, plancheck.ResourceActionReplace),
				Check:            checkGotifyOnServer(te, newName, server, "", false),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceGotifyNotificationDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("gotify")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name   = "{{.Name}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             gotifyCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.DeleteGotifyTarget(te.Context(), name)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           config,
				ConfigPlanChecks: expectAction(gotifyResource, plancheck.ResourceActionCreate),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceGotifyNotificationImportServerCreated(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("gotify")
	server := te.RenderConfig("http://{{.GotifyHost}}:{{.GotifyPort}}")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_gotify_notification" "test" {
  name   = "{{.Name}}"
  server = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token  = "tfacc-token"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             gotifyCheckDestroy(te),
		Steps: []resource.TestStep{
			importServerCreatedStep(t, gotifyResource, name, config,
				func() error {
					return te.Client().Notifications.CreateGotifyTarget(te.Context(), &notifications.GotifyTarget{Name: name, Server: server, Token: "tfacc-token"})
				},
				func() error { return te.Client().Notifications.DeleteGotifyTarget(context.Background(), name) },
				map[string]string{"name": name, "server": server, "disable": "false"},
			),
		},
	})
}
