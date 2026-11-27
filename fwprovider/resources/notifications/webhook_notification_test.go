/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/notifications"
)

const webhookResource = "pbs_webhook_notification.test"

func webhookCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_webhook_notification", "name", func(id string) error {
		_, err := te.Client().Notifications.GetWebhookTarget(te.Context(), id)
		return err
	})
}

func checkWebhookOnServer(te *test.Environment, want notifications.WebhookTarget) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := te.Client().Notifications.GetWebhookTarget(te.Context(), want.Name)
		if err != nil {
			return err
		}

		var headerErr error
		if !maps.Equal(got.Headers, want.Headers) && (len(got.Headers) != 0 || len(want.Headers) != 0) {
			headerErr = errors.New("API header mismatch")
		}

		return errors.Join(
			checkEqual("url", got.URL, want.URL),
			checkEqual("method", got.Method, want.Method),
			checkEqual("body", got.Body, want.Body),
			checkEqual("comment", got.Comment, want.Comment),
			checkEqual("disable", boolOrFalse(got.Disable), boolOrFalse(want.Disable)),
			headerErr,
		)
	}
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceWebhookNotification(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("webhook")
	newName := te.RandomName("webhook")
	url := te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify")
	te.AddTemplateVars(map[string]any{"Name": name, "NewName": newName})

	disabled := true

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             webhookCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name    = "{{.Name}}"
  url     = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
  comment = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "name", name),
					resource.TestCheckResourceAttr(webhookResource, "url", url),
					resource.TestCheckResourceAttr(webhookResource, "method", "post"),
					resource.TestCheckResourceAttr(webhookResource, "disable", "false"),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:    name,
						URL:     url,
						Method:  "post",
						Comment: "created by acceptance test",
					}),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name    = "{{.Name}}"
  url     = "http://{{.WebhookHost}}:{{.WebhookPort}}/updated"
  method  = "put"
  comment = "updated by acceptance test"
  disable = true
}`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "method", "put"),
					resource.TestCheckResourceAttr(webhookResource, "disable", "true"),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:    name,
						URL:     te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/updated"),
						Method:  "put",
						Comment: "updated by acceptance test",
						Disable: &disabled,
					}),
				),
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name = "{{.Name}}"
  url  = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
}`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "method", "post"),
					resource.TestCheckNoResourceAttr(webhookResource, "comment"),
					resource.TestCheckResourceAttr(webhookResource, "disable", "false"),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:   name,
						URL:    url,
						Method: "post",
					}),
				),
			},
			{
				ResourceName:                         webhookResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name = "{{.NewName}}"
  url  = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
}`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionReplace),
				Check: checkWebhookOnServer(te, notifications.WebhookTarget{
					Name:   newName,
					URL:    url,
					Method: "post",
				}),
			},
		},
	})
}

// TestAccResourceWebhookNotificationBodyAndHeaders covers the attributes PBS
// stores base64-encoded (body, header values).
//
//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceWebhookNotificationBodyAndHeaders(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("webhook")
	te.AddTemplateVars(map[string]any{"Name": name})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             webhookCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name    = "{{.Name}}"
  url     = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
  body    = "{\"title\": \"{{"{{"}} title {{"}}"}}\"}"
  headers = {
    "Content-Type" = "application/json"
  }
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "headers.Content-Type", "application/json"),
					resource.TestCheckResourceAttr(webhookResource, "body", `{"title": "{{ title }}"}`),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:    name,
						URL:     te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify"),
						Method:  "post",
						Body:    `{"title": "{{ title }}"}`,
						Headers: map[string]string{"Content-Type": "application/json"},
					}),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name    = "{{.Name}}"
  url     = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
  body    = "{\"message\": \"{{"{{"}} message {{"}}"}}\"}"
  headers = {
    "Content-Type" = "application/json"
    "X-Test"       = "1"
  }
}`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "headers.%", "2"),
					resource.TestCheckResourceAttr(webhookResource, "body", `{"message": "{{ message }}"}`),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:    name,
						URL:     te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify"),
						Method:  "post",
						Body:    `{"message": "{{ message }}"}`,
						Headers: map[string]string{"Content-Type": "application/json", "X-Test": "1"},
					}),
				),
			},
			{
				ResourceName:                         webhookResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name = "{{.Name}}"
  url  = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
}`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(webhookResource, "body"),
					resource.TestCheckNoResourceAttr(webhookResource, "headers.%"),
					checkWebhookOnServer(te, notifications.WebhookTarget{
						Name:   name,
						URL:    te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify"),
						Method: "post",
					}),
				),
			},
		},
	})
}

// checkWebhookSecretNames verifies the secret names configured on PBS. PBS
// only returns secret names, never values.
func checkWebhookSecretNames(te *test.Environment, name string, want ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := te.Client().Notifications.GetWebhookTarget(te.Context(), name)
		if err != nil {
			return err
		}

		names := slices.Clone(got.SecretNames)
		slices.Sort(names)
		slices.Sort(want)

		return checkSlice("secret names", names, want)
	}
}

// TestAccResourceWebhookNotificationSecret covers the secret map, which PBS
// stores as named, base64-encoded values referenced as {{ secrets.<name> }}.
//
//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceWebhookNotificationSecret(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("webhook")
	te.AddTemplateVars(map[string]any{"Name": name})

	body := `{"token": "{{ secrets.token }}"}`
	withSecrets := func(secrets string) string {
		return te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name   = "{{.Name}}"
  url    = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
  body   = "{\"token\": \"{{"{{"}} secrets.token {{"}}"}}\"}"
` + secrets + `
}`)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             webhookCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: withSecrets(`  secret = { token = "s3cr3t" }`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "secret.%", "1"),
					resource.TestCheckResourceAttr(webhookResource, "secret.token", "s3cr3t"),
					resource.TestCheckResourceAttr(webhookResource, "body", body),
					checkWebhookSecretNames(te, name, "token"),
				),
			},
			{
				// Changing only a secret value must update in place.
				Config:           withSecrets(`  secret = { token = "rotated" }`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "secret.token", "rotated"),
					checkWebhookSecretNames(te, name, "token"),
				),
			},
			{
				Config: withSecrets(`  secret = {
    token = "rotated"
    other = "x"
  }`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "secret.%", "2"),
					checkWebhookSecretNames(te, name, "other", "token"),
				),
			},
			{
				Config:           withSecrets(`  secret = { token = "rotated" }`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResource, "secret.%", "1"),
					resource.TestCheckNoResourceAttr(webhookResource, "secret.other"),
					checkWebhookSecretNames(te, name, "token"),
				),
			},
			{
				ResourceName:                         webhookResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// PBS returns secret names but never their values.
				ImportStateVerifyIgnore: []string{"secret"},
			},
			{
				// Secrets removed outside Terraform must show up as drift.
				Config: withSecrets(`  secret = { token = "rotated" }`),
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.UpdateWebhookTarget(te.Context(), name, &notifications.WebhookTarget{
						Delete: []string{"secret"},
					})
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           withSecrets(`  secret = { token = "rotated" }`),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check:            checkWebhookSecretNames(te, name, "token"),
			},
			{
				// Removing the whole map must clear the secrets on the server.
				Config:           withSecrets(``),
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(webhookResource, "secret.%"),
					checkWebhookSecretNames(te, name),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceWebhookNotificationDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("webhook")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name = "{{.Name}}"
  url  = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             webhookCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.DeleteWebhookTarget(te.Context(), name)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           config,
				ConfigPlanChecks: expectAction(webhookResource, plancheck.ResourceActionCreate),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceWebhookNotificationImportServerCreated(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("webhook")
	url := te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_webhook_notification" "test" {
  name = "{{.Name}}"
  url  = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             webhookCheckDestroy(te),
		Steps: []resource.TestStep{
			importServerCreatedStep(t, webhookResource, name, config,
				func() error {
					return te.Client().Notifications.CreateWebhookTarget(te.Context(), &notifications.WebhookTarget{Name: name, URL: url, Method: "post"})
				},
				func() error { return te.Client().Notifications.DeleteWebhookTarget(context.Background(), name) },
				map[string]string{"name": name, "url": url, "method": "post", "disable": "false"},
			),
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}
