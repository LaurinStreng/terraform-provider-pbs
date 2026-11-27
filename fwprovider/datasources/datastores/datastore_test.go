/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package datastores_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

const dataSourceName = "data.pbs_datastore.test"

//testacc:tier=medium
//testacc:resource=datastore
func TestAccDataSourceDatastore(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{"Name": name, "Path": te.DatastorePath(name)})

	config := `
resource "pbs_datastore" "test" {
  name        = "{{.Name}}"
  path        = "{{.Path}}"
  comment     = "%s"
  gc_schedule = "%s"
  verify_new  = true
}

data "pbs_datastore" "test" {
  name = pbs_datastore.test.name
}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(fmt.Sprintf(config, "data source test", "daily")),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "name", name),
					resource.TestCheckResourceAttr(dataSourceName, "path", te.DatastorePath(name)),
					resource.TestCheckResourceAttr(dataSourceName, "comment", "data source test"),
					resource.TestCheckResourceAttr(dataSourceName, "gc_schedule", "daily"),
					resource.TestCheckResourceAttr(dataSourceName, "verify_new", "true"),
					resource.TestCheckResourceAttr(dataSourceName, "removable", "false"),
					resource.TestCheckNoResourceAttr(dataSourceName, "s3_client"),
					resource.TestCheckNoResourceAttr(dataSourceName, "s3_bucket"),
				),
			},
			{
				// The data source must reflect changes made to the datastore.
				Config: te.RenderConfig(fmt.Sprintf(config, "data source test updated", "weekly")),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "comment", "data source test updated"),
					resource.TestCheckResourceAttr(dataSourceName, "gc_schedule", "weekly"),
				),
			},
		},
	})
}

//testacc:tier=medium
//testacc:resource=datastore
func TestAccDataSourceDatastoreDigest(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{"Name": name, "Path": te.DatastorePath(name)})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_datastore" "test" {
  name = "{{.Name}}"
  path = "{{.Path}}"
}

data "pbs_datastore" "test" {
  name = pbs_datastore.test.name
}`),
				Check: resource.TestMatchResourceAttr(dataSourceName, "digest", regexp.MustCompile(`^[0-9a-f]{64}$`)),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=datastore
func TestAccDataSourceDatastoreNotFound(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddTemplateVars(map[string]any{"Name": te.RandomName("ds")})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
data "pbs_datastore" "test" {
  name = "{{.Name}}"
}`),
				ExpectError: regexp.MustCompile(`Error Reading Datastore`),
			},
		},
	})
}
