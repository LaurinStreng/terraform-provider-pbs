/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

// IsNotFound reports whether err is a PBS "object does not exist" error.
func IsNotFound(err error) bool {
	return api.IsNotFound(err)
}

// ExistsFunc fetches an object by its ID; it must return an error satisfying
// IsNotFound when the object is absent.
type ExistsFunc func(id string) error

// CheckDestroy returns a resource.TestCase CheckDestroy function that asserts
// every resource of resourceType in state is gone from the PBS server.
// idAttr selects the state attribute holding the object ID (usually "name"
// or "id").
func CheckDestroy(resourceType, idAttr string, get ExistsFunc) func(*terraform.State) error {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}

			id := rs.Primary.Attributes[idAttr]

			err := get(id)
			if err == nil {
				return fmt.Errorf("%s %q still exists on the server", resourceType, id)
			}

			if !IsNotFound(err) {
				return fmt.Errorf("checking %s %q: %w", resourceType, id, err)
			}
		}

		return nil
	}
}

// ImportStateIDFromAttr returns an ImportStateIdFunc that uses the given
// attribute of resourceName as import ID.
func ImportStateIDFromAttr(resourceName, attr string) func(*terraform.State) (string, error) {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("resource %s not found in state", resourceName)
		}

		return rs.Primary.Attributes[attr], nil
	}
}

// SkipUnlessEnv skips the test unless all given environment variables are
// set. Use it for tests that need optional external services (e.g. S3).
func SkipUnlessEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, k := range keys {
		if os.Getenv(k) == "" {
			t.Skipf("%s not set", k)
		}
	}
}
