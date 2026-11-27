/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package endpoints_test

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/assert"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/datasources/endpoints"
	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

// TestS3EndpointDataSourceSchema verifies the s3_endpoint data source schema
func TestS3EndpointDataSourceSchema(t *testing.T) {
	ds := endpoints.NewS3EndpointDataSource()

	schemaReq := datasource.SchemaRequest{}
	schemaResp := &datasource.SchemaResponse{}

	ds.Schema(context.Background(), schemaReq, schemaResp)

	assert.NotNil(t, schemaResp.Schema)
	assert.NotEmpty(t, schemaResp.Schema.Description)

	// Verify required attributes
	idAttr, exists := schemaResp.Schema.Attributes["id"]
	assert.True(t, exists, "id attribute should exist")
	assert.True(t, idAttr.(schema.StringAttribute).Required, "id should be required")

	// Verify computed attributes
	accessKeyAttr, exists := schemaResp.Schema.Attributes["access_key"]
	assert.True(t, exists, "access_key attribute should exist")
	assert.True(t, accessKeyAttr.(schema.StringAttribute).Computed, "access_key should be computed")
	assert.True(t, accessKeyAttr.(schema.StringAttribute).Sensitive, "access_key should be sensitive")

	endpointAttr, exists := schemaResp.Schema.Attributes["endpoint"]
	assert.True(t, exists, "endpoint attribute should exist")
	assert.True(t, endpointAttr.(schema.StringAttribute).Computed, "endpoint should be computed")

	regionAttr, exists := schemaResp.Schema.Attributes["region"]
	assert.True(t, exists, "region attribute should exist")
	assert.True(t, regionAttr.(schema.StringAttribute).Computed, "region should be computed")

	fingerprintAttr, exists := schemaResp.Schema.Attributes["fingerprint"]
	assert.True(t, exists, "fingerprint attribute should exist")
	assert.True(t, fingerprintAttr.(schema.StringAttribute).Computed, "fingerprint should be computed")

	portAttr, exists := schemaResp.Schema.Attributes["port"]
	assert.True(t, exists, "port attribute should exist")
	assert.True(t, portAttr.(schema.Int64Attribute).Computed, "port should be computed")

	pathStyleAttr, exists := schemaResp.Schema.Attributes["path_style"]
	assert.True(t, exists, "path_style attribute should exist")
	assert.True(t, pathStyleAttr.(schema.BoolAttribute).Computed, "path_style should be computed")

	providerQuirksAttr, exists := schemaResp.Schema.Attributes["provider_quirks"]
	assert.True(t, exists, "provider_quirks attribute should exist")
	assert.True(t, providerQuirksAttr.(schema.SetAttribute).Computed, "provider_quirks should be computed")
}

// TestS3EndpointsDataSourceSchema verifies the s3_endpoints data source schema
func TestS3EndpointsDataSourceSchema(t *testing.T) {
	ds := endpoints.NewS3EndpointsDataSource()

	schemaReq := datasource.SchemaRequest{}
	schemaResp := &datasource.SchemaResponse{}

	ds.Schema(context.Background(), schemaReq, schemaResp)

	assert.NotNil(t, schemaResp.Schema)
	assert.NotEmpty(t, schemaResp.Schema.Description)

	// Verify endpoints list attribute
	endpointsAttr, exists := schemaResp.Schema.Attributes["endpoints"]
	assert.True(t, exists, "endpoints attribute should exist")
	assert.True(t, endpointsAttr.(schema.ListNestedAttribute).Computed, "endpoints should be computed")

	// Verify nested attributes in the endpoints list
	nestedObj := endpointsAttr.(schema.ListNestedAttribute).NestedObject
	assert.NotNil(t, nestedObj.Attributes)

	// Check all nested attributes exist and are computed
	nestedAttrs := []string{"id", "access_key", "endpoint", "region", "fingerprint", "port", "path_style", "provider_quirks"}
	for _, attrName := range nestedAttrs {
		_, exists := nestedObj.Attributes[attrName]
		assert.True(t, exists, "%s attribute should exist in nested object", attrName)
	}
}

const s3EndpointsConfig = `
resource "pbs_s3_endpoint" "one" {
  id              = "{{.ID1}}"
  endpoint        = "s3.example.invalid"
  region          = "us-east-1"
  port            = 9000
  access_key      = "tfacc-access-key-1"
  secret_key      = "tfacc-secret-key-1"
  path_style      = true
  provider_quirks = ["skip-if-none-match-header"]
}

resource "pbs_s3_endpoint" "two" {
  id         = "{{.ID2}}"
  endpoint   = "minio.example.invalid"
  access_key = "tfacc-access-key-2"
  secret_key = "tfacc-secret-key-2"
}
`

// PBS stores S3 endpoint configurations without contacting the endpoint, so
// dummy hosts and credentials are sufficient here.

//testacc:tier=light
//testacc:resource=endpoints
func TestAccDataSourceS3Endpoint(t *testing.T) {
	te := test.InitEnvironment(t)
	te.AddTemplateVars(map[string]any{"ID1": te.RandomName("s3"), "ID2": te.RandomName("s3")})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(s3EndpointsConfig + `
data "pbs_s3_endpoint" "one" {
  id = pbs_s3_endpoint.one.id
}

data "pbs_s3_endpoint" "two" {
  id = pbs_s3_endpoint.two.id
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.pbs_s3_endpoint.one", "id", "pbs_s3_endpoint.one", "id"),
					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.one", "endpoint", "s3.example.invalid"),
					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.one", "region", "us-east-1"),
					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.one", "port", "9000"),
					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.one", "access_key", "tfacc-access-key-1"),
					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.one", "path_style", "true"),
					resource.TestCheckTypeSetElemAttr("data.pbs_s3_endpoint.one", "provider_quirks.*", "skip-if-none-match-header"),

					resource.TestCheckResourceAttr("data.pbs_s3_endpoint.two", "endpoint", "minio.example.invalid"),
					resource.TestCheckNoResourceAttr("data.pbs_s3_endpoint.two", "region"),
					resource.TestCheckNoResourceAttr("data.pbs_s3_endpoint.two", "port"),
					resource.TestCheckNoResourceAttr("data.pbs_s3_endpoint.two", "provider_quirks.#"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=endpoints
func TestAccDataSourceS3Endpoints(t *testing.T) {
	te := test.InitEnvironment(t)

	id1 := te.RandomName("s3")
	id2 := te.RandomName("s3")
	te.AddTemplateVars(map[string]any{"ID1": id1, "ID2": id2})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(s3EndpointsConfig + `
data "pbs_s3_endpoints" "all" {
  depends_on = [pbs_s3_endpoint.one, pbs_s3_endpoint.two]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_s3_endpoints.all", "endpoints.*", map[string]string{
						"id":                id1,
						"endpoint":          "s3.example.invalid",
						"region":            "us-east-1",
						"port":              "9000",
						"path_style":        "true",
						"provider_quirks.#": "1",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_s3_endpoints.all", "endpoints.*", map[string]string{
						"id":         id2,
						"endpoint":   "minio.example.invalid",
						"access_key": "tfacc-access-key-2",
					}),
				),
			},
		},
	})
}
