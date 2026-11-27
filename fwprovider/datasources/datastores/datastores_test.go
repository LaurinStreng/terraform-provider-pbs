/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package datastores_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/datasources/datastores"
	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

func TestDatastoresDataSourceSchema(t *testing.T) {
	ds := datastores.NewDatastoresDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}

	ds.Schema(context.Background(), req, resp)

	require.False(t, resp.Diagnostics.HasError())
	require.NotNil(t, resp.Schema.Attributes)

	// Verify stores attribute exists and is computed
	storesAttr, ok := resp.Schema.Attributes["stores"]
	require.True(t, ok, "stores attribute should exist")
	require.True(t, storesAttr.IsComputed(), "stores should be computed")
}

//testacc:tier=medium
//testacc:resource=datastore
func TestAccDataSourceDatastores(t *testing.T) {
	te := test.InitEnvironment(t)

	name1 := te.RandomName("ds")
	name2 := te.RandomName("ds")
	te.AddTemplateVars(map[string]any{
		"Name1": name1, "Path1": te.DatastorePath(name1),
		"Name2": name2, "Path2": te.DatastorePath(name2),
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_datastore" "one" {
  name        = "{{.Name1}}"
  path        = "{{.Path1}}"
  comment     = "first data source test datastore"
  gc_schedule = "daily"
}

resource "pbs_datastore" "two" {
  name    = "{{.Name2}}"
  path    = "{{.Path2}}"
  comment = "second data source test datastore"
}

data "pbs_datastores" "all" {
  depends_on = [pbs_datastore.one, pbs_datastore.two]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_datastores.all", "stores.*", map[string]string{
						"name":        name1,
						"path":        te.DatastorePath(name1),
						"comment":     "first data source test datastore",
						"gc_schedule": "daily",
						"removable":   "false",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_datastores.all", "stores.*", map[string]string{
						"name":    name2,
						"path":    te.DatastorePath(name2),
						"comment": "second data source test datastore",
					}),
				),
			},
		},
	})
}
