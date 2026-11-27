/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// expectAction asserts the planned action for resourceName before apply.
func expectAction(resourceName string, action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(resourceName, action),
		},
	}
}

func checkEqual[T comparable](field string, got, want T) error {
	if got != want {
		return fmt.Errorf("API %s = %v, want %v", field, got, want)
	}

	return nil
}

func checkSlice(field string, got, want []string) error {
	if !slices.Equal(got, want) {
		return fmt.Errorf("API %s = %v, want %v", field, got, want)
	}

	return nil
}

func boolOrFalse(b *bool) bool {
	return b != nil && *b
}

// importServerCreatedStep returns a test step that creates an object directly
// via the API (with only required fields, so PBS stores no optional values),
// imports it, and checks that omitted values were mapped to schema defaults.
func importServerCreatedStep(
	t *testing.T,
	resourceName, id, config string,
	create, cleanup func() error,
	want map[string]string,
) resource.TestStep {
	return resource.TestStep{
		PreConfig: func() {
			if err := create(); err != nil {
				t.Fatalf("creating %s via API: %v", id, err)
			}

			t.Cleanup(func() { _ = cleanup() })
		},
		Config:             config,
		ResourceName:       resourceName,
		ImportState:        true,
		ImportStateId:      id,
		ImportStatePersist: true,
		ImportStateCheck: func(states []*terraform.InstanceState) error {
			if len(states) != 1 {
				return fmt.Errorf("expected 1 imported instance, got %d", len(states))
			}

			var errs []error

			for k, v := range want {
				if got, ok := states[0].Attributes[k]; !ok || got != v {
					errs = append(errs, fmt.Errorf("imported %s = %q (present: %t), want %q", k, got, ok, v))
				}
			}

			return errors.Join(errs...)
		},
	}
}
