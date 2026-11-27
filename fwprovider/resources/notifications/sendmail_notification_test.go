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

const sendmailResource = "pbs_sendmail_notification.test"

func sendmailCheckDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_sendmail_notification", "name", func(id string) error {
		_, err := te.Client().Notifications.GetSendmailTarget(te.Context(), id)
		return err
	})
}

func checkSendmailOnServer(te *test.Environment, want notifications.SendmailTarget) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := te.Client().Notifications.GetSendmailTarget(te.Context(), want.Name)
		if err != nil {
			return err
		}

		return errors.Join(
			checkEqual("from-address", got.From, want.From),
			checkSlice("mailto", got.Mailto, want.Mailto),
			checkSlice("mailto-user", got.MailtoUser, want.MailtoUser),
			checkEqual("author", got.Author, want.Author),
			checkEqual("comment", got.Comment, want.Comment),
			checkEqual("disable", boolOrFalse(got.Disable), boolOrFalse(want.Disable)),
		)
	}
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSendmailNotification(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("sendmail")
	newName := te.RandomName("sendmail")
	te.AddTemplateVars(map[string]any{"Name": name, "NewName": newName})

	disabled := true

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             sendmailCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.Name}}"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
  author       = "PBS Test"
  comment      = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(sendmailResource, "name", name),
					resource.TestCheckResourceAttr(sendmailResource, "from_address", "pbs@example.com"),
					resource.TestCheckResourceAttr(sendmailResource, "mailto.#", "1"),
					resource.TestCheckResourceAttr(sendmailResource, "mailto.0", "admin@example.com"),
					resource.TestCheckResourceAttr(sendmailResource, "author", "PBS Test"),
					resource.TestCheckResourceAttr(sendmailResource, "disable", "false"),
					checkSendmailOnServer(te, notifications.SendmailTarget{
						Name:    name,
						From:    "pbs@example.com",
						Mailto:  []string{"admin@example.com"},
						Author:  "PBS Test",
						Comment: "created by acceptance test",
					}),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.Name}}"
  from_address = "backup@example.com"
  mailto       = ["admin@example.com", "ops@example.com"]
  mailto_user  = ["root@pam"]
  author       = "PBS Updated"
  comment      = "updated by acceptance test"
  disable      = true
}`),
				ConfigPlanChecks: expectAction(sendmailResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(sendmailResource, "mailto.#", "2"),
					resource.TestCheckResourceAttr(sendmailResource, "mailto_user.0", "root@pam"),
					resource.TestCheckResourceAttr(sendmailResource, "disable", "true"),
					checkSendmailOnServer(te, notifications.SendmailTarget{
						Name:       name,
						From:       "backup@example.com",
						Mailto:     []string{"admin@example.com", "ops@example.com"},
						MailtoUser: []string{"root@pam"},
						Author:     "PBS Updated",
						Comment:    "updated by acceptance test",
						Disable:    &disabled,
					}),
				),
			},
			{
				// Removing optional attributes must clear them on the server.
				Config: te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.Name}}"
  from_address = "backup@example.com"
  mailto       = ["ops@example.com"]
}`),
				ConfigPlanChecks: expectAction(sendmailResource, plancheck.ResourceActionUpdate),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(sendmailResource, "mailto_user.#"),
					resource.TestCheckNoResourceAttr(sendmailResource, "author"),
					resource.TestCheckNoResourceAttr(sendmailResource, "comment"),
					resource.TestCheckResourceAttr(sendmailResource, "disable", "false"),
					checkSendmailOnServer(te, notifications.SendmailTarget{
						Name:   name,
						From:   "backup@example.com",
						Mailto: []string{"ops@example.com"},
					}),
				),
			},
			{
				ResourceName:                         sendmailResource,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.NewName}}"
  from_address = "backup@example.com"
  mailto       = ["ops@example.com"]
}`),
				ConfigPlanChecks: expectAction(sendmailResource, plancheck.ResourceActionReplace),
				Check: checkSendmailOnServer(te, notifications.SendmailTarget{
					Name:   newName,
					From:   "backup@example.com",
					Mailto: []string{"ops@example.com"},
				}),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSendmailNotificationDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("sendmail")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.Name}}"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             sendmailCheckDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					return te.Client().Notifications.DeleteSendmailTarget(te.Context(), name)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config:           config,
				ConfigPlanChecks: expectAction(sendmailResource, plancheck.ResourceActionCreate),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=notifications
func TestAccResourceSendmailNotificationImportServerCreated(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("sendmail")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_sendmail_notification" "test" {
  name         = "{{.Name}}"
  from_address = "pbs@example.com"
  mailto       = ["admin@example.com"]
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             sendmailCheckDestroy(te),
		Steps: []resource.TestStep{
			importServerCreatedStep(t, sendmailResource, name, config,
				func() error {
					return te.Client().Notifications.CreateSendmailTarget(te.Context(), &notifications.SendmailTarget{
						Name: name, From: "pbs@example.com", Mailto: []string{"admin@example.com"},
					})
				},
				func() error { return te.Client().Notifications.DeleteSendmailTarget(context.Background(), name) },
				map[string]string{"name": name, "mailto.0": "admin@example.com", "disable": "false"},
			),
			{
				// Nothing differs from the configuration, so the plan must be empty.
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}
