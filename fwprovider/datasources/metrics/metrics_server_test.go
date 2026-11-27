/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package metrics_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/datasources/metrics"
	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider/test"
)

func TestMetricsServerDataSource(t *testing.T) {
	t.Parallel()

	ds := metrics.NewMetricsServerDataSource()

	if ds == nil {
		t.Fatal("NewMetricsServerDataSource() returned nil")
	}

	if _, ok := ds.(datasource.DataSourceWithConfigure); !ok {
		t.Fatal("NewMetricsServerDataSource() did not return a datasource.DataSourceWithConfigure")
	}
}

func TestMetricsServersDataSource(t *testing.T) {
	t.Parallel()

	ds := metrics.NewMetricsServersDataSource()

	if ds == nil {
		t.Fatal("NewMetricsServersDataSource() returned nil")
	}

	if _, ok := ds.(datasource.DataSourceWithConfigure); !ok {
		t.Fatal("NewMetricsServersDataSource() did not return a datasource.DataSourceWithConfigure")
	}
}

const metricsServersConfig = `
resource "pbs_metrics_server" "http" {
  name          = "{{.HTTPName}}"
  type          = "influxdb-http"
  url           = "http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"
  organization  = "{{.InfluxDBOrg}}"
  bucket        = "{{.InfluxDBBucket}}"
  token         = "{{.InfluxDBToken}}"
  max_body_size = 65536
  comment       = "http data source test"
}

resource "pbs_metrics_server" "udp" {
  name    = "{{.UDPName}}"
  type    = "influxdb-udp"
  server  = "{{.InfluxDBUDPHost}}"
  port    = {{.InfluxDBUDPPort}}
  mtu     = 1400
  enable  = false
  comment = "udp data source test"
}
`

//testacc:tier=light
//testacc:resource=metrics
func TestAccDataSourceMetricsServer(t *testing.T) {
	te := test.InitEnvironment(t)

	httpName := te.RandomName("influx")
	udpName := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"HTTPName": httpName, "UDPName": udpName})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(metricsServersConfig + `
data "pbs_metrics_server" "http" {
  name = pbs_metrics_server.http.name
  type = pbs_metrics_server.http.type
}

data "pbs_metrics_server" "udp" {
  name = pbs_metrics_server.udp.name
  type = pbs_metrics_server.udp.type
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "name", httpName),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "type", "influxdb-http"),
					resource.TestCheckResourceAttrPair("data.pbs_metrics_server.http", "url", "pbs_metrics_server.http", "url"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "server", te.RenderConfig("{{.InfluxDBHost}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "port", te.RenderConfig("{{.InfluxDBPort}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "organization", te.RenderConfig("{{.InfluxDBOrg}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "bucket", te.RenderConfig("{{.InfluxDBBucket}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "max_body_size", "65536"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "verify_tls", "true"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "enable", "true"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.http", "comment", "http data source test"),

					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "name", udpName),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "type", "influxdb-udp"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "server", te.RenderConfig("{{.InfluxDBUDPHost}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "port", te.RenderConfig("{{.InfluxDBUDPPort}}")),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "mtu", "1400"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "enable", "false"),
					resource.TestCheckResourceAttr("data.pbs_metrics_server.udp", "comment", "udp data source test"),
				),
			},
		},
	})
}

//testacc:tier=light
//testacc:resource=metrics
func TestAccDataSourceMetricsServers(t *testing.T) {
	te := test.InitEnvironment(t)

	httpName := te.RandomName("influx")
	udpName := te.RandomName("influx")
	te.AddTemplateVars(map[string]any{"HTTPName": httpName, "UDPName": udpName})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: te.ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: te.RenderConfig(metricsServersConfig + `
data "pbs_metrics_servers" "all" {
  depends_on = [pbs_metrics_server.http, pbs_metrics_server.udp]
}`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_metrics_servers.all", "servers.*", map[string]string{
						"name":          httpName,
						"type":          "influxdb-http",
						"url":           te.RenderConfig("http://{{.InfluxDBHost}}:{{.InfluxDBPort}}"),
						"organization":  te.RenderConfig("{{.InfluxDBOrg}}"),
						"bucket":        te.RenderConfig("{{.InfluxDBBucket}}"),
						"max_body_size": "65536",
						"verify_tls":    "true",
						"enable":        "true",
						"comment":       "http data source test",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.pbs_metrics_servers.all", "servers.*", map[string]string{
						"name":    udpName,
						"type":    "influxdb-udp",
						"server":  te.RenderConfig("{{.InfluxDBUDPHost}}"),
						"port":    te.RenderConfig("{{.InfluxDBUDPPort}}"),
						"mtu":     "1400",
						"enable":  "false",
						"comment": "udp data source test",
					}),
				),
			},
		},
	})
}
