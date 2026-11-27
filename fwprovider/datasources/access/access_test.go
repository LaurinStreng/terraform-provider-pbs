/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package access_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/datasources/access"
	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

func TestDataSourceConstructors(t *testing.T) {
	t.Parallel()

	for name, ctor := range map[string]func() datasource.DataSource{
		"user":  access.NewUserDataSource,
		"users": access.NewUsersDataSource,
		"acls":  access.NewACLsDataSource,
		"roles": access.NewRolesDataSource,
	} {
		if _, ok := ctor().(datasource.DataSourceWithConfigure); !ok {
			t.Errorf("%s data source does not implement datasource.DataSourceWithConfigure", name)
		}
	}
}

const usersConfig = `
resource "pbs_user" "full" {
  userid    = "{{.FullUser}}"
  comment   = "data source test"
  firstname = "Terra"
  lastname  = "Form"
  email     = "tfacc@example.com"
  enable    = false
  expire    = 4102444800
}

resource "pbs_user" "minimal" {
  userid = "{{.MinimalUser}}"
}
`

//testacc:tier=light
//testacc:resource=access
func TestAccDataSourceUser(t *testing.T) {
	te := test.InitEnvironment(t)

	fullUser := te.RandomName("user") + "@pbs"
	minimalUser := te.RandomName("user") + "@pbs"
	te.AddTemplateVars(map[string]any{"FullUser": fullUser, "MinimalUser": minimalUser})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(usersConfig + `
data "pbs_user" "full" {
  userid = pbs_user.full.userid
}

data "pbs_user" "minimal" {
  userid = pbs_user.minimal.userid
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_user.full", "userid", fullUser),
					resource.TestCheckResourceAttr("data.pbs_user.full", "comment", "data source test"),
					resource.TestCheckResourceAttr("data.pbs_user.full", "firstname", "Terra"),
					resource.TestCheckResourceAttr("data.pbs_user.full", "lastname", "Form"),
					resource.TestCheckResourceAttr("data.pbs_user.full", "email", "tfacc@example.com"),
					resource.TestCheckResourceAttr("data.pbs_user.full", "enable", "false"),
					resource.TestCheckResourceAttr("data.pbs_user.full", "expire", "4102444800"),

					// PBS omits defaults; the data source must map them back.
					resource.TestCheckResourceAttr("data.pbs_user.minimal", "userid", minimalUser),
					resource.TestCheckResourceAttr("data.pbs_user.minimal", "enable", "true"),
					resource.TestCheckResourceAttr("data.pbs_user.minimal", "expire", "0"),
					resource.TestCheckNoResourceAttr("data.pbs_user.minimal", "comment"),
					resource.TestCheckNoResourceAttr("data.pbs_user.minimal", "firstname"),
					resource.TestCheckNoResourceAttr("data.pbs_user.minimal", "lastname"),
					resource.TestCheckNoResourceAttr("data.pbs_user.minimal", "email"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccDataSourceUserNotFound(t *testing.T) {
	te := test.InitEnvironment(t)

	te.AddTemplateVars(map[string]any{"Missing": te.RandomName("user") + "@pbs"})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
data "pbs_user" "missing" {
  userid = "{{.Missing}}"
}`),
				ExpectError: regexp.MustCompile(`Error Reading User`),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccDataSourceUsers(t *testing.T) {
	te := test.InitEnvironment(t)

	fullUser := te.RandomName("user") + "@pbs"
	minimalUser := te.RandomName("user") + "@pbs"
	te.AddTemplateVars(map[string]any{"FullUser": fullUser, "MinimalUser": minimalUser})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(usersConfig + `
data "pbs_users" "all" {
  depends_on = [pbs_user.full, pbs_user.minimal]
}

data "pbs_users" "pbs" {
  realm      = "pbs"
  depends_on = [pbs_user.full, pbs_user.minimal]
}

data "pbs_users" "pam" {
  realm      = "pam"
  depends_on = [pbs_user.full, pbs_user.minimal]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.all", "users.*", map[string]string{
						"userid":    fullUser,
						"comment":   "data source test",
						"firstname": "Terra",
						"lastname":  "Form",
						"email":     "tfacc@example.com",
						"enable":    "false",
						"expire":    "4102444800",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.all", "users.*", map[string]string{
						"userid": minimalUser,
						"enable": "true",
						"expire": "0",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.all", "users.*", map[string]string{
						"userid": "root@pam",
					}),

					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.pbs", "users.*", map[string]string{
						"userid": fullUser,
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.pbs", "users.*", map[string]string{
						"userid": minimalUser,
					}),
					checkAllUsersInRealm("data.pbs_users.pbs", "pbs"),

					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_users.pam", "users.*", map[string]string{
						"userid": "root@pam",
					}),
					checkAllUsersInRealm("data.pbs_users.pam", "pam"),
				),
			},
		},
	})
}

// checkAllUsersInRealm verifies that every user listed by a pbs_users data
// source belongs to realm.
func checkAllUsersInRealm(name, realm string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		attrs := rs.Primary.Attributes
		n, err := strconv.Atoi(attrs["users.#"])
		if err != nil {
			return fmt.Errorf("%s: invalid users.#: %w", name, err)
		}
		if n == 0 {
			return fmt.Errorf("%s: no users returned for realm %s", name, realm)
		}
		for i := 0; i < n; i++ {
			id := attrs[fmt.Sprintf("users.%d.userid", i)]
			if !strings.HasSuffix(id, "@"+realm) {
				return fmt.Errorf("%s: user %q is not in realm %s", name, id, realm)
			}
		}
		return nil
	}
}

//testacc:tier=light
//testacc:resource=access
func TestAccDataSourceACLs(t *testing.T) {
	te := test.InitEnvironment(t)

	user := te.RandomName("user") + "@pbs"
	path := "/datastore/" + te.RandomName("ds")
	te.AddTemplateVars(map[string]any{"User": user, "Path": path})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_user" "test" {
  userid = "{{.User}}"
}

resource "pbs_acl" "backup" {
  path      = "{{.Path}}"
  auth_id   = pbs_user.test.userid
  role      = "DatastoreBackup"
  propagate = false
}

resource "pbs_acl" "audit" {
  path    = "{{.Path}}/ns1"
  auth_id = pbs_user.test.userid
  role    = "DatastoreAudit"
}

data "pbs_acls" "exact" {
  path       = "{{.Path}}"
  exact      = true
  depends_on = [pbs_acl.backup, pbs_acl.audit]
}

data "pbs_acls" "subtree" {
  path       = "{{.Path}}"
  depends_on = [pbs_acl.backup, pbs_acl.audit]
}

data "pbs_acls" "by_user" {
  auth_id    = pbs_user.test.userid
  depends_on = [pbs_acl.backup, pbs_acl.audit]
}

data "pbs_acls" "other_user" {
  path       = "{{.Path}}"
  auth_id    = "root@pam"
  depends_on = [pbs_acl.backup, pbs_acl.audit]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_acls.exact", "acls.#", "1"),
					resource.TestCheckResourceAttr("data.pbs_acls.exact", "acls.0.path", path),
					resource.TestCheckResourceAttr("data.pbs_acls.exact", "acls.0.auth_id", user),
					resource.TestCheckResourceAttr("data.pbs_acls.exact", "acls.0.role", "DatastoreBackup"),
					resource.TestCheckResourceAttr("data.pbs_acls.exact", "acls.0.propagate", "false"),

					resource.TestCheckResourceAttr("data.pbs_acls.subtree", "acls.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_acls.subtree", "acls.*", map[string]string{
						"path":      path + "/ns1",
						"auth_id":   user,
						"role":      "DatastoreAudit",
						"propagate": "true",
					}),

					resource.TestCheckResourceAttr("data.pbs_acls.by_user", "acls.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_acls.by_user", "acls.*", map[string]string{
						"path":      path,
						"auth_id":   user,
						"role":      "DatastoreBackup",
						"propagate": "false",
					}),

					resource.TestCheckResourceAttr("data.pbs_acls.other_user", "acls.#", "0"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=access
func TestAccDataSourceRoles(t *testing.T) {
	te := test.InitEnvironment(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "pbs_roles" "all" {}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_roles.all", "roles.*", map[string]string{
						"role_id": "Admin",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_roles.all", "roles.*", map[string]string{
						"role_id": "DatastoreBackup",
					}),
					checkRolePrivilege("data.pbs_roles.all", "DatastoreBackup", "Datastore.Backup"),
					checkRolePrivilege("data.pbs_roles.all", "Admin", "Sys.Modify"),
				),
			},
		},
	})
}

// checkRolePrivilege verifies that role is listed with privilege priv.
func checkRolePrivilege(name, role, priv string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("%s not found in state", name)
		}
		attrs := rs.Primary.Attributes
		n, _ := strconv.Atoi(attrs["roles.#"])
		for i := 0; i < n; i++ {
			if attrs[fmt.Sprintf("roles.%d.role_id", i)] != role {
				continue
			}
			np, _ := strconv.Atoi(attrs[fmt.Sprintf("roles.%d.privileges.#", i)])
			for j := 0; j < np; j++ {
				if attrs[fmt.Sprintf("roles.%d.privileges.%d", i, j)] == priv {
					return nil
				}
			}
			return fmt.Errorf("%s: role %s does not have privilege %s", name, role, priv)
		}
		return fmt.Errorf("%s: role %s not found", name, role)
	}
}
