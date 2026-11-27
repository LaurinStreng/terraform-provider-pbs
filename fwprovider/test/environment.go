/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

// Package test contains shared helpers for acceptance tests.
//
// Acceptance tests run against a real Proxmox Backup Server. They are only
// executed when TF_ACC=1 is set; use the ./testacc script in the repository
// root, which loads credentials from testacc.env.
package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"

	"github.com/LaurinStreng/terraform-provider-pbs/fwprovider"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs"
	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

// NamePrefix is prepended to every object created by acceptance tests, so
// leftovers from aborted runs are easy to identify and clean up.
const NamePrefix = "tfacc"

// Environment bundles everything an acceptance test needs: provider
// factories, a direct API client for out-of-band verification, and
// template variables for rendering HCL configurations.
type Environment struct {
	t *testing.T

	// ProviderFactories wires the in-process provider into terraform-plugin-testing.
	ProviderFactories map[string]func() (tfprotov6.ProviderServer, error)

	templateVars map[string]any

	clientOnce sync.Once
	client     *pbs.Client
	clientErr  error
}

// InitEnvironment creates a new test environment. It is safe to call in
// unit-test mode (TF_ACC unset); the API client is only created on first use.
func InitEnvironment(t *testing.T) *Environment {
	t.Helper()

	return &Environment{
		t: t,
		ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"pbs": providerserver.NewProtocol6WithError(fwprovider.New("test")()),
		},
		templateVars: map[string]any{
			"InfluxDBHost":    envOr("PBS_TEST_INFLUXDB_HOST", "influxdb"),
			"InfluxDBPort":    envOr("PBS_TEST_INFLUXDB_PORT", "8086"),
			"InfluxDBOrg":     envOr("PBS_TEST_INFLUXDB_ORG", "testorg"),
			"InfluxDBBucket":  envOr("PBS_TEST_INFLUXDB_BUCKET", "pbs-metrics"),
			"InfluxDBToken":   envOr("PBS_TEST_INFLUXDB_TOKEN", "test-token-for-pbs-provider"),
			"InfluxDBUDPHost": envOr("PBS_TEST_INFLUXDB_UDP_HOST", "influxdb-udp"),
			"InfluxDBUDPPort": envOr("PBS_TEST_INFLUXDB_UDP_PORT", "8089"),
			"GotifyHost":      envOr("PBS_TEST_GOTIFY_HOST", "gotify"),
			"GotifyPort":      envOr("PBS_TEST_GOTIFY_PORT", "80"),
			"WebhookHost":     envOr("PBS_TEST_WEBHOOK_HOST", "webhook"),
			"WebhookPort":     envOr("PBS_TEST_WEBHOOK_PORT", "8080"),
			"DatastoreRoot":   strings.TrimRight(envOr("PBS_TEST_DATASTORE_ROOT", "/datastore"), "/"),
		},
	}
}

// AddTemplateVars adds or overrides variables available in RenderConfig.
func (e *Environment) AddTemplateVars(vars map[string]any) {
	for k, v := range vars {
		e.templateVars[k] = v
	}
}

// RenderConfig renders an HCL configuration using Go text/template syntax,
// e.g. `name = "{{.Name}}"`. The test fails if rendering fails.
func (e *Environment) RenderConfig(cfg string) string {
	e.t.Helper()

	tmpl, err := template.New("config").Option("missingkey=error").Parse(cfg)
	if err != nil {
		e.t.Fatalf("parsing config template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, e.templateVars); err != nil {
		e.t.Fatalf("rendering config template: %v", err)
	}

	return buf.String()
}

// RandomName returns a unique, PBS-safe object name such as "tfacc-ds-a1b2c3d4".
// The result starts with a letter and only contains lowercase letters, digits
// and hyphens, which satisfies every PBS ID format used by this provider.
func (e *Environment) RandomName(kind string) string {
	return fmt.Sprintf("%s-%s-%s", NamePrefix, kind, strings.ToLower(acctest.RandString(8)))
}

// DatastorePath returns the filesystem path for a test datastore.
func (e *Environment) DatastorePath(name string) string {
	return fmt.Sprintf("%s/%s", e.templateVars["DatastoreRoot"], name)
}

// Client returns a PBS API client configured from the same environment
// variables as the provider. Use it to verify state directly on the server.
func (e *Environment) Client() *pbs.Client {
	e.t.Helper()

	e.clientOnce.Do(func() {
		e.client, e.clientErr = pbs.NewClient(
			api.Credentials{
				Username: os.Getenv("PBS_USERNAME"),
				Password: os.Getenv("PBS_PASSWORD"),
				APIToken: os.Getenv("PBS_API_TOKEN"),
			},
			api.ClientOptions{
				Endpoint: os.Getenv("PBS_ENDPOINT"),
				Insecure: os.Getenv("PBS_INSECURE") == "true",
				Timeout:  60 * time.Second,
			},
		)
	})

	if e.clientErr != nil {
		e.t.Fatalf("creating PBS API client: %v", e.clientErr)
	}

	return e.client
}

// Context returns a context bounded by the test deadline (or 5 minutes).
func (e *Environment) Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if deadline, ok := e.t.Deadline(); ok {
		cancel()
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	}

	e.t.Cleanup(cancel)

	return ctx
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
