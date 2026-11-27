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

const smtpResource = "pbs_smtp_notification.test"

func smtpCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_smtp_notification", "name", func(id string) error {
		_, err := te.Client().Notifications.GetSMTPTarget(te.Context(), id)
		return err
	})
}

// checkSMTPOnServer compares the server-side SMTP target with want. A nil
// want.Port means PBS should report the default port 25 (or omit it).
func checkSMTPOnServer(te *test.Environment, want notifications.SMTPTarget) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := te.Client().Notifications.GetSMTPTarget(te.Context(), want.Name)
		if err != nil {
			return err
		}

		gotPort, wantPort := 25, 25
		if got.Port != nil {
			gotPort = *got.Port
		}

		if want.Port != nil {
			wantPort = *want.Port
		}

		return errors.Join(
			checkEqual("server", got.Server, want.Server),
			checkEqual("port", gotPort, wantPort),
			checkEqual("mode", got.Mode, want.Mode),
			checkEqual("from-address", got.From, want.From),
			checkSlice("mailto", got.To, want.To),
			checkSlice("mailto-user", got.MailtoUser, want.MailtoUser),
			checkEqual("username", got.Username, want.Username),
			checkEqual("author", got.Author, want.Author),
			checkEqual("comment", got.Comment, want.Comment),
			checkEqual("disable", boolOrFalse(got.Disable), boolOrFalse(want.Disable)),
		)
	}
}

func intPtr(i int) *int { return &i }

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSMTPNotification(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("smtp")
	newName := te.RandomName("smtp")
	te.AddTemplateVars(map[string]any{"Name": name, "NewName": newName})

	disabled := true

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             smtpCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "smtp.example.com"
  port         = 587
  mode         = "starttls"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
  username     = "smtp-user"
  password     = "secret123"
  author       = "PBS Test"
  comment      = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(smtpResource, "name", name),
					resource.TestCheckResourceAttr(smtpResource, "server", "smtp.example.com"),
					resource.TestCheckResourceAttr(smtpResource, "port", "587"),
					resource.TestCheckResourceAttr(smtpResource, "mode", "starttls"),
					resource.TestCheckResourceAttr(smtpResource, "username", "smtp-user"),
					resource.TestCheckResourceAttr(smtpResource, "disable", "false"),
					checkSMTPOnServer(te, notifications.SMTPTarget{
						Name:     name,
						Server:   "smtp.example.com",
						Port:     intPtr(587),
						Mode:     "starttls",
						From:     "pbs@example.com",
						To:       []string{"admin@example.com"},
						Username: "smtp-user",
						Author:   "PBS Test",
						Comment:  "created by acceptance test",
					}),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "mail.example.com"
  port         = 465
  mode         = "tls"
  from_address = "backup@example.com"
  mailto       = ["admin@example.com", "ops@example.com"]
  mailto_user  = ["root@pam"]
  username     = "smtp-user2"
  password     = "secret456"
  author       = "PBS Updated"
  comment      = "updated by acceptance test"
  disable      = true
}`),
				ConfigPlanChecks: expectAction(smtpResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(smtpResource, "port", "465"),
					resource.TestCheckResourceAttr(smtpResource, "mode", "tls"),
					resource.TestCheckResourceAttr(smtpResource, "mailto.#", "2"),
					resource.TestCheckResourceAttr(smtpResource, "disable", "true"),
					checkSMTPOnServer(te, notifications.SMTPTarget{
						Name:       name,
						Server:     "mail.example.com",
						Port:       intPtr(465),
						Mode:       "tls",
						From:       "backup@example.com",
						To:         []string{"admin@example.com", "ops@example.com"},
						MailtoUser: []string{"root@pam"},
						Username:   "smtp-user2",
						Author:     "PBS Updated",
						Comment:    "updated by acceptance test",
						Disable:    &disabled,
					}),
				),
			},
			{
				// Removing optional attributes must clear them on the server
				// and fall back to the schema defaults for port and mode.
				Config: te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "mail.example.com"
  from_address = "backup@example.com"
  mailto_user  = ["root@pam"]
}`),
				ConfigPlanChecks: expectAction(smtpResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(smtpResource, "port", "25"),
					resource.TestCheckResourceAttr(smtpResource, "mode", "insecure"),
					resource.TestCheckNoResourceAttr(smtpResource, "mailto.#"),
					resource.TestCheckNoResourceAttr(smtpResource, "username"),
					resource.TestCheckNoResourceAttr(smtpResource, "author"),
					resource.TestCheckNoResourceAttr(smtpResource, "comment"),
					resource.TestCheckResourceAttr(smtpResource, "disable", "false"),
					checkSMTPOnServer(te, notifications.SMTPTarget{
						Name:       name,
						Server:     "mail.example.com",
						Mode:       "insecure",
						From:       "backup@example.com",
						MailtoUser: []string{"root@pam"},
					}),
				),
			},
			{
				ResourceName:                         smtpResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.NewName}}"
  server       = "mail.example.com"
  from_address = "backup@example.com"
  mailto_user  = ["root@pam"]
}`),
				ConfigPlanChecks: expectAction(smtpResource, plancheck.ResourceActionReplace),
				Check: checkSMTPOnServer(te, notifications.SMTPTarget{
					Name:       newName,
					Server:     "mail.example.com",
					Mode:       "insecure",
					From:       "backup@example.com",
					MailtoUser: []string{"root@pam"},
				}),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSMTPNotificationImportWithPassword(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("smtp")
	te.AddTemplateVars(map[string]any{"Name": name})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             smtpCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "smtp.example.com"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
  username     = "smtp-user"
  password     = "secret123"
}`),
			},
			{
				ResourceName:                         smtpResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// PBS never returns the SMTP password.
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSMTPNotificationDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("smtp")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "smtp.example.com"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             smtpCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.DeleteSMTPTarget(te.Context(), name)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           config,
				ConfigPlanChecks: expectAction(smtpResource, plancheck.ResourceActionCreate),
			},
		},
	})
}

// TestAccResourceSMTPNotificationImportServerCreated imports an SMTP target
// that PBS stores without mode/port. PBS then uses mode "tls" and the mode's
// default port 465, which state must reflect.
//
//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSMTPNotificationImportServerCreated(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("smtp")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_smtp_notification" "test" {
  name         = "{{.Name}}"
  server       = "smtp.example.com"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
  mode         = "tls"
  port         = 465
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             smtpCheckDestroy(te),
		Steps: []resource.TestStep{
			importServerCreatedStep(t, smtpResource, name, config,
				func() error {
					return te.Client().Notifications.CreateSMTPTarget(te.Context(), &notifications.SMTPTarget{
						Name: name, Server: "smtp.example.com", From: "pbs@example.com", To: []string{"admin@example.com"},
					})
				},
				func() error { return te.Client().Notifications.DeleteSMTPTarget(context.Background(), name) },
				map[string]string{"name": name, "mode": "tls", "port": "465", "disable": "false"},
			),
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}
