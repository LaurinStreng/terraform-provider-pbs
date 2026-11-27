/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package metrics_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/metrics"
)

const resourceName = "pbs_metrics_server.test"

// checkDestroy asserts that no metrics server from state survives on PBS.
// The API path depends on the server type, which is stored in state.
func checkDestroy(te *test.Environment) func(*terraform.State) error {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "pbs_metrics_server" {
				continue
			}

			name := rs.Primary.Attributes["name"]
			serverType := metrics.MetricsServerType(rs.Primary.Attributes["type"])

			_, err := te.Client().Metrics.GetMetricsServer(te.Context(), serverType, name)
			if err == nil {
				return fmt.Errorf("metrics server %s/%s still exists on the server", serverType, name)
			}

			if !test.IsNotFound(err) {
				return fmt.Errorf("checking metrics server %s/%s: %w", serverType, name, err)
			}
		}

		return nil
	}
}

// checkServerOnAPI reads the metrics server directly from PBS and hands it to
// fn for verification, independent of what the provider wrote to state.
func checkServerOnAPI(
	te *test.Environment,
	serverType metrics.MetricsServerType,
	name string,
	fn func(*metrics.MetricsServer) error,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		srv, err := te.Client().Metrics.GetMetricsServer(te.Context(), serverType, name)
		if err != nil {
			return fmt.Errorf("reading metrics server %s/%s from API: %w", serverType, name, err)
		}

		return fn(srv)
	}
}

func intPtrEquals(field string, got *int, want int) error {
	if got == nil {
		return fmt.Errorf("API %s = <unset>, want %d", field, want)
	}

	if *got != want {
		return fmt.Errorf("API %s = %d, want %d", field, *got, want)
	}

	return nil
}

func boolPtrEquals(field string, got *bool, want bool) error {
	// PBS omits boolean flags that are at their default value.
	if got == nil {
		if want {
			return nil
		}

		return fmt.Errorf("API %s = <unset>, want false", field)
	}

	if *got != want {
		return fmt.Errorf("API %s = %t, want %t", field, *got, want)
	}

	return nil
}

//testacc:tier=light
//testacc:resource=metrics
func TestAccResourceMetricsServerInfluxDBHTTP(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"Name": name, "NewName": name + "-r"})
	url := te.RenderConfig("http://{{.InfluxDBHost}}:{{.InfluxDBPort}}")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name         = "{{.Name}}"
  type         = "influxdb-http"
  url          = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization = "{{.InfluxDBOrg}}"
  bucket       = "{{.InfluxDBBucket}}"
  token        = "{{.InfluxDBToken}}"
  comment      = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "type", "influxdb-http"),
					resource.TestCheckResourceAttr(resourceName, "url", url),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					resource.TestCheckResourceAttr(resourceName, "verify_tls", "true"),
					resource.TestCheckResourceAttr(resourceName, "max_body_size", "25000000"),
					resource.TestCheckResourceAttr(resourceName, "comment", "created by acceptance test"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name, func(s *metrics.MetricsServer) error {
						if s.URL != url {
							return fmt.Errorf("API url = %q, want %q", s.URL, url)
						}
						if s.Comment != "created by acceptance test" {
							return fmt.Errorf("API comment = %q", s.Comment)
						}
						return boolPtrEquals("enable", s.Enable, true)
					}),
				),
			},
			{
				// Mutable fields must be updated in place.
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name          = "{{.Name}}"
  type          = "influxdb-http"
  url           = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization  = "{{.InfluxDBOrg}}"
  bucket        = "{{.InfluxDBBucket}}"
  token         = "{{.InfluxDBToken}}"
  enable        = false
  verify_tls    = false
  max_body_size = 65536
  comment       = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					resource.TestCheckResourceAttr(resourceName, "verify_tls", "false"),
					resource.TestCheckResourceAttr(resourceName, "max_body_size", "65536"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name, func(s *metrics.MetricsServer) error {
						if s.Comment != "updated by acceptance test" {
							return fmt.Errorf("API comment = %q", s.Comment)
						}
						if err := boolPtrEquals("enable", s.Enable, false); err != nil {
							return err
						}
						if err := boolPtrEquals("verify-tls", s.VerifyTLS, false); err != nil {
							return err
						}
						return intPtrEquals("max-body-size", s.MaxBodySize, 65536)
					}),
				),
			},
			{
				// Going back to defaults and dropping the comment must clear
				// the values on the server, not only in state.
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name         = "{{.Name}}"
  type         = "influxdb-http"
  url          = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization = "{{.InfluxDBOrg}}"
  bucket       = "{{.InfluxDBBucket}}"
  token        = "{{.InfluxDBToken}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceName, "comment"),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name, func(s *metrics.MetricsServer) error {
						if s.Comment != "" {
							return fmt.Errorf("API comment = %q, want empty", s.Comment)
						}
						if err := boolPtrEquals("enable", s.Enable, true); err != nil {
							return err
						}
						return boolPtrEquals("verify-tls", s.VerifyTLS, true)
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        "influxdb-http/" + name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// PBS never returns the InfluxDB token.
				ImportStateVerifyIgnore: []string{"token"},
			},
			{
				// The name is the PBS ID and cannot be changed in place.
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name         = "{{.NewName}}"
  type         = "influxdb-http"
  url          = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization = "{{.InfluxDBOrg}}"
  bucket       = "{{.InfluxDBBucket}}"
  token        = "{{.InfluxDBToken}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name+"-r"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name+"-r", func(*metrics.MetricsServer) error {
						return nil
					}),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=metrics
func TestAccResourceMetricsServerInfluxDBHTTPServerAndPort(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"Name": name})
	url := te.RenderConfig("http://{{.InfluxDBHost}}:{{.InfluxDBPort}}")

	// server+port is the legacy alternative to url; the provider builds the
	// URL itself. The default post-apply plan check catches perpetual diffs.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name         = "{{.Name}}"
  type         = "influxdb-http"
  server       = "{{.InfluxDBHost}}"
  port         = {{.InfluxDBPort}}
  organization = "{{.InfluxDBOrg}}"
  bucket       = "{{.InfluxDBBucket}}"
  token        = "{{.InfluxDBToken}}"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "server", te.RenderConfig("{{.InfluxDBHost}}")),
					resource.TestCheckResourceAttr(resourceName, "port", te.RenderConfig("{{.InfluxDBPort}}")),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name, func(s *metrics.MetricsServer) error {
						if s.URL != url {
							return fmt.Errorf("API url = %q, want %q", s.URL, url)
						}
						return nil
					}),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=metrics
func TestAccResourceMetricsServerInfluxDBUDP(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"Name": name})
	host := te.RenderConfig("{{.InfluxDBUDPHost}}:{{.InfluxDBUDPPort}}")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name    = "{{.Name}}"
  type    = "influxdb-udp"
  server  = "{{.InfluxDBUDPHost}}"
  port    = {{.InfluxDBUDPPort}}
  comment = "created by acceptance test"
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "type", "influxdb-udp"),
					resource.TestCheckResourceAttr(resourceName, "server", te.RenderConfig("{{.InfluxDBUDPHost}}")),
					resource.TestCheckResourceAttr(resourceName, "port", te.RenderConfig("{{.InfluxDBUDPPort}}")),
					resource.TestCheckResourceAttr(resourceName, "mtu", "1500"),
					resource.TestCheckResourceAttr(resourceName, "enable", "true"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBUDP, name, func(s *metrics.MetricsServer) error {
						if s.Host != host {
							return fmt.Errorf("API host = %q, want %q", s.Host, host)
						}
						if s.Comment != "created by acceptance test" {
							return fmt.Errorf("API comment = %q", s.Comment)
						}
						return nil
					}),
				),
			},
			{
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name    = "{{.Name}}"
  type    = "influxdb-udp"
  server  = "{{.InfluxDBUDPHost}}"
  port    = {{.InfluxDBUDPPort}}
  mtu     = 1400
  enable  = false
  comment = "updated by acceptance test"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "mtu", "1400"),
					resource.TestCheckResourceAttr(resourceName, "enable", "false"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBUDP, name, func(s *metrics.MetricsServer) error {
						if s.Comment != "updated by acceptance test" {
							return fmt.Errorf("API comment = %q", s.Comment)
						}
						if err := intPtrEquals("mtu", s.MTU, 1400); err != nil {
							return err
						}
						return boolPtrEquals("enable", s.Enable, false)
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateId:                        "influxdb-udp/" + name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				// The server type selects a different PBS API collection and
				// therefore forces replacement.
				Config: te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name         = "{{.Name}}"
  type         = "influxdb-http"
  url          = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization = "{{.InfluxDBOrg}}"
  bucket       = "{{.InfluxDBBucket}}"
  token        = "{{.InfluxDBToken}}"
}`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "type", "influxdb-http"),
					checkServerOnAPI(te, metrics.MetricsServerTypeInfluxDBHTTP, name, func(*metrics.MetricsServer) error {
						return nil
					}),
					func(_ *terraform.State) error {
						// Metrics server names share one namespace across types,
						// so PBS answers with a type mismatch instead of "not found".
						if _, err := te.Client().Metrics.GetMetricsServer(te.Context(), metrics.MetricsServerTypeInfluxDBUDP, name); err == nil {
							return fmt.Errorf("old influxdb-udp server %q still exists", name)
						}
						return nil
					},
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=metrics
func TestAccResourceMetricsServerDeletedOutOfBand(t *testing.T) {
	te := test.InitEnvironment(t)

	name := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"Name": name})

	config := te.RenderConfig(`
resource "pbs_metrics_server" "test" {
  name   = "{{.Name}}"
  type   = "influxdb-udp"
  server = "{{.InfluxDBUDPHost}}"
  port   = {{.InfluxDBUDPPort}}
}`)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		CheckDestroy:             checkDestroy(te),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(_ *terraform.State) error {
					// Remove the metrics server behind Terraform's back.
					return te.Client().Metrics.DeleteMetricsServer(te.Context(), metrics.MetricsServerTypeInfluxDBUDP, name)
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
