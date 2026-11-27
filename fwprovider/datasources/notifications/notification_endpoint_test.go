/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// endpointsConfig declares one endpoint of every type.
const endpointsConfig = `
resource "pbs_gotify_notification" "test" {
  name    = "{{.GotifyName}}"
  server  = "http://{{.GotifyHost}}:{{.GotifyPort}}"
  token   = "tfacc-token"
  comment = "gotify endpoint"
}

resource "pbs_smtp_notification" "test" {
  name         = "{{.SMTPName}}"
  server       = "smtp.example.com"
  port         = 587
  mode         = "starttls"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
  username     = "smtp-user"
  password     = "secret123"
  author       = "PBS Test"
  comment      = "smtp endpoint"
}

resource "pbs_sendmail_notification" "test" {
  name         = "{{.SendmailName}}"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com", "ops@example.com"]
  mailto_user  = ["root@pam"]
  author       = "PBS Test"
  comment      = "sendmail endpoint"
  disable      = true
}

resource "pbs_webhook_notification" "test" {
  name    = "{{.WebhookName}}"
  url     = "http://{{.WebhookHost}}:{{.WebhookPort}}/notify"
  method  = "put"
  body    = "hello"
  headers = {
    "X-Test" = "1"
  }
  secret = {
    token = "s3cr3t"
  }
  comment = "webhook endpoint"
}
`

func endpointNames(te *test.Environment) map[string]any {
	vars := map[string]any{
		"GotifyName":   te.RandomName("gotify"),
		"SMTPName":     te.RandomName("smtp"),
		"SendmailName": te.RandomName("sendmail"),
		"WebhookName":  te.RandomName("webhook"),
	}
	te.AddTemplateVars(vars)

	return vars
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationEndpoint(t *testing.T) {
	te := test.InitEnvironment(t)
	names := endpointNames(te)
	webhookURL := te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify")
	gotifyURL := te.RenderConfig("http://{{.GotifyHost}}:{{.GotifyPort}}")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(endpointsConfig + `
data "pbs_notification_endpoint" "gotify" {
  name = pbs_gotify_notification.test.name
  type = "gotify"
}

data "pbs_notification_endpoint" "smtp" {
  name = pbs_smtp_notification.test.name
  type = "smtp"
}

data "pbs_notification_endpoint" "sendmail" {
  name = pbs_sendmail_notification.test.name
  type = "sendmail"
}

data "pbs_notification_endpoint" "webhook" {
  name = pbs_webhook_notification.test.name
  type = "webhook"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.gotify", "name", names["GotifyName"].(string)),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.gotify", "url", gotifyURL),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.gotify", "comment", "gotify endpoint"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.gotify", "disable", "false"),
					resource.TestCheckNoResourceAttr("data.pbs_notification_endpoint.gotify", "token"),

					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "server", "smtp.example.com"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "port", "587"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "mode", "starttls"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "from_address", "pbs@example.com"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "mailto.#", "1"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "mailto.0", "admin@example.com"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "username", "smtp-user"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "author", "PBS Test"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.smtp", "comment", "smtp endpoint"),

					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.sendmail", "from_address", "pbs@example.com"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.sendmail", "mailto.#", "2"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.sendmail", "mailto_user.0", "root@pam"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.sendmail", "author", "PBS Test"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.sendmail", "disable", "true"),

					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "url", webhookURL),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "method", "put"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "body", "hello"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "headers.X-Test", "1"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "comment", "webhook endpoint"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "secret_names.#", "1"),
					resource.TestCheckResourceAttr("data.pbs_notification_endpoint.webhook", "secret_names.0", "token"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationEndpointNotFound(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddTemplateVars(map[string]any{"Name": te.RandomName("missing")})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
data "pbs_notification_endpoint" "test" {
  name = "{{.Name}}"
  type = "gotify"
}`),
				ExpectError: regexp.MustCompile(`(?i)not found|does not exist|no such`),
			},
			{
				Config: te.RenderConfig(`
data "pbs_notification_endpoint" "test" {
  name = "{{.Name}}"
  type = "pager"
}`),
				ExpectError: regexp.MustCompile(`(?i)unknown endpoint type`),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccDataSourceNotificationEndpoints(t *testing.T) {
	te := test.InitEnvironment(t)
	names := endpointNames(te)

	const ds = "data.pbs_notification_endpoints.all"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(endpointsConfig + `
data "pbs_notification_endpoints" "all" {
  depends_on = [
    pbs_gotify_notification.test,
    pbs_smtp_notification.test,
    pbs_sendmail_notification.test,
    pbs_webhook_notification.test,
  ]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs(ds, "endpoints.*", map[string]string{
						"name":    names["GotifyName"].(string),
						"type":    "gotify",
						"url":     te.RenderConfig("http://{{.GotifyHost}}:{{.GotifyPort}}"),
						"comment": "gotify endpoint",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "endpoints.*", map[string]string{
						"name":         names["SMTPName"].(string),
						"type":         "smtp",
						"server":       "smtp.example.com",
						"port":         "587",
						"mode":         "starttls",
						"from_address": "pbs@example.com",
						"mailto.#":     "1",
						"username":     "smtp-user",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "endpoints.*", map[string]string{
						"name":          names["SendmailName"].(string),
						"type":          "sendmail",
						"from_address":  "pbs@example.com",
						"mailto.#":      "2",
						"mailto_user.0": "root@pam",
						"disable":       "true",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(ds, "endpoints.*", map[string]string{
						"name":           names["WebhookName"].(string),
						"type":           "webhook",
						"url":            te.RenderConfig("http://{{.WebhookHost}}:{{.WebhookPort}}/notify"),
						"method":         "put",
						"body":           "hello",
						"headers.X-Test": "1",
					}),
				),
			},
		},
	})
}
