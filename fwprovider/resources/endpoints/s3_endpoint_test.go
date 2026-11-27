/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package endpoints_test

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/endpoints"
)

const resourceName = "pbs_s3_endpoint.test"

// testFingerprint is a syntactically valid SHA-256 fingerprint. PBS does not
// connect to the endpoint when the configuration is saved, so the value does
// not need to match a real certificate.
const testFingerprint = "AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99"

func checkDestroy(te *test.Environment) func(*terraform.State) error {
	return test.CheckDestroy("pbs_s3_endpoint", "id", func(id string) error {
		_, err := te.Client().Endpoints.GetS3Endpoint(te.Context(), id)
		return err
	})
}

// checkEndpointOnAPI reads the S3 endpoint directly from PBS and hands it to
// fn for verification, independent of what the provider wrote to state.
func checkEndpointOnAPI(te *test.Environment, id string, fn func(*endpoints.S3Endpoint) error) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ep, err := te.Client().Endpoints.GetS3Endpoint(te.Context(), id)
		if err != nil {
			return fmt.Errorf("reading S3 endpoint %q from API: %w", id, err)
		}

		return fn(ep)
	}
}

// PBS stores S3 endpoint configurations without contacting the endpoint, so
// the light-tier tests below use dummy hosts and credentials.

//testacc:tier=light
//testacc:resource=endpoints
func TestAccResourceS3Endpoint(t *testing.T) {
	te := test.InitEnvironment(t)

	id := te.RandomName("s3")
	te.AddTemplateVars(map[string]any{"ID": id, "NewID": id + "-r", "Fingerprint": testFingerprint})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_s3_endpoint" "test" {
  id         = "{{.ID}}"
  endpoint   = "s3.example.invalid"
  region     = "us-east-1"
  access_key = "tfacc-access-key"
  secret_key = "tfacc-secret-key"
  path_style = true
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", id),
					resource.TestCheckResourceAttr(resourceName, "endpoint", "s3.example.invalid"),
					resource.TestCheckResourceAttr(resourceName, "region", "us-east-1"),
					resource.TestCheckResourceAttr(resourceName, "path_style", "true"),
					checkEndpointOnAPI(te, id, func(ep *endpoints.S3Endpoint) error {
						if ep.Endpoint != "s3.example.invalid" || ep.Region != "us-east-1" || ep.AccessKey != "tfacc-access-key" {
							return fmt.Errorf("API endpoint/region/access-key = %q/%q/%q", ep.Endpoint, ep.Region, ep.AccessKey)
						}
						if ep.PathStyle == nil || !*ep.PathStyle {
							return fmt.Errorf("API path-style = %v, want true", ep.PathStyle)
						}
						return nil
					}),
				),
			},
			{
				// All connection settings are mutable and must be updated in place.
				Config: te.RenderConfig(`
resource "pbs_s3_endpoint" "test" {
  id              = "{{.ID}}"
  endpoint        = "minio.example.invalid"
  region          = "eu-central-1"
  port            = 9000
  access_key      = "tfacc-access-key-2"
  secret_key      = "tfacc-secret-key-2"
  path_style      = false
  fingerprint     = "{{.Fingerprint}}"
  provider_quirks = ["skip-if-none-match-header"]
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "endpoint", "minio.example.invalid"),
					resource.TestCheckResourceAttr(resourceName, "port", "9000"),
					resource.TestCheckResourceAttr(resourceName, "provider_quirks.#", "1"),
					checkEndpointOnAPI(te, id, func(ep *endpoints.S3Endpoint) error {
						if ep.Endpoint != "minio.example.invalid" || ep.Region != "eu-central-1" || ep.AccessKey != "tfacc-access-key-2" {
							return fmt.Errorf("API endpoint/region/access-key = %q/%q/%q", ep.Endpoint, ep.Region, ep.AccessKey)
						}
						if ep.Port == nil || *ep.Port != 9000 {
							return fmt.Errorf("API port = %v, want 9000", ep.Port)
						}
						if ep.PathStyle != nil && *ep.PathStyle {
							return fmt.Errorf("API path-style = true, want false")
						}
						if ep.Fingerprint != testFingerprint {
							return fmt.Errorf("API fingerprint = %q, want %q", ep.Fingerprint, testFingerprint)
						}
						if !slices.Equal(ep.ProviderQuirks, []string{"skip-if-none-match-header"}) {
							return fmt.Errorf("API provider-quirks = %v", ep.ProviderQuirks)
						}
						return nil
					}),
				),
			},
			{
				// Dropping optional settings must clear them on the server.
				Config: te.RenderConfig(`
resource "pbs_s3_endpoint" "test" {
  id         = "{{.ID}}"
  endpoint   = "minio.example.invalid"
  access_key = "tfacc-access-key-2"
  secret_key = "tfacc-secret-key-2"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "region"),
					resource.TestCheckNoResourceAttr(resourceName, "port"),
					resource.TestCheckNoResourceAttr(resourceName, "fingerprint"),
					checkEndpointOnAPI(te, id, func(ep *endpoints.S3Endpoint) error {
						if ep.Region != "" || ep.Port != nil || ep.Fingerprint != "" || len(ep.ProviderQuirks) > 0 || ep.PathStyle != nil {
							return fmt.Errorf("API still has optional settings: region=%q port=%v fingerprint=%q quirks=%v path-style=%v",
								ep.Region, ep.Port, ep.Fingerprint, ep.ProviderQuirks, ep.PathStyle)
						}
						return nil
					}),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateId:     id,
				ImportStateVerify: true,
				// PBS never returns the secret key.
				ImportStateVerifyIgnore: []string{"secret_key"},
			},
			{
				// The ID is the PBS key and cannot be changed in place.
				Config: te.RenderConfig(`
resource "pbs_s3_endpoint" "test" {
  id         = "{{.NewID}}"
  endpoint   = "minio.example.invalid"
  access_key = "tfacc-access-key-2"
  secret_key = "tfacc-secret-key-2"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", id+"-r"),
					checkEndpointOnAPI(te, id+"-r", func(*endpoints.S3Endpoint) error { return nil }),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=endpoints
func TestAccResourceS3EndpointDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	id := te.RandomName("s3")
	te.AddTemplateVars(map[string]any{"ID": id})

	config := te.RenderConfig(`
resource "pbs_s3_endpoint" "test" {
  id         = "{{.ID}}"
  endpoint   = "s3.example.invalid"
  access_key = "tfacc-access-key"
  secret_key = "tfacc-secret-key"
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the endpoint behind Terraform's back.
					return te.Client().Endpoints.DeleteS3Endpoint(te.Context(), id)
				},
				// Refresh must drop the resource from state and plan a re-create.
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
					},
				},
			},
		},
	})
}

// TestAccResourceS3EndpointWithDatastore creates an S3-backed datastore on a
// real bucket. It needs working S3 credentials and is skipped otherwise.
//
//testacc:tier=heavy
//testacc:resource=endpoints,datastore
func TestAccResourceS3EndpointWithDatastore(t *testing.T) {
	test.SkipUnlessEnv(t, "PBS_TEST_S3_ENDPOINT", "PBS_TEST_S3_ACCESS_KEY", "PBS_TEST_S3_SECRET_KEY", "PBS_TEST_S3_BUCKET")

	te := test.InitEnvironment(t)

	id := te.RandomName("s3")
	dsName := te.RandomName("ds")
	region := os.Getenv("PBS_TEST_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}

	te.AddTemplateVars(map[string]any{
		"ID":        id,
		"DSName":    dsName,
		"DSPath":    te.DatastorePath(dsName),
		"Endpoint":  os.Getenv("PBS_TEST_S3_ENDPOINT"),
		"Region":    region,
		"AccessKey": os.Getenv("PBS_TEST_S3_ACCESS_KEY"),
		"SecretKey": os.Getenv("PBS_TEST_S3_SECRET_KEY"),
		"Bucket":    os.Getenv("PBS_TEST_S3_BUCKET"),
	})

	configTmpl := `
resource "pbs_s3_endpoint" "test" {
  id         = "{{.ID}}"
  endpoint   = "{{.Endpoint}}"
  region     = "{{.Region}}"
  access_key = "{{.AccessKey}}"
  secret_key = "{{.SecretKey}}"
  path_style = true
}

resource "pbs_datastore" "test" {
  name      = "{{.DSName}}"
  path      = "{{.DSPath}}"
  s3_client = pbs_s3_endpoint.test.id
  s3_bucket = "{{.Bucket}}"
  comment   = "%s"
}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy: resource.ComposeTestCheckFunc(
			checkDestroy(te),
			test.CheckDestroy("pbs_datastore", "name", func(name string) error {
				_, err := te.Client().Datastores.GetDatastore(te.Context(), name)
				return err
			}),
		),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(fmt.Sprintf(configTmpl, "created by acceptance test")),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair("pbs_datastore.test", "s3_client", resourceName, "id"),
					resource.TestCheckResourceAttr("pbs_datastore.test", "s3_bucket", os.Getenv("PBS_TEST_S3_BUCKET")),
					func(_ *terraform.State) error {
						ds, err := te.Client().Datastores.GetDatastore(te.Context(), dsName)
						if err != nil {
							return fmt.Errorf("reading datastore %q from API: %w", dsName, err)
						}
						if ds.S3Client != id || ds.S3Bucket != os.Getenv("PBS_TEST_S3_BUCKET") {
							return fmt.Errorf("API s3 client/bucket = %q/%q", ds.S3Client, ds.S3Bucket)
						}
						return nil
					},
				),
			},
			{
				Config: te.RenderConfig(fmt.Sprintf(configTmpl, "updated by acceptance test")),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("pbs_datastore.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr("pbs_datastore.test", "comment", "updated by acceptance test"),
			},
		},
	})
}
