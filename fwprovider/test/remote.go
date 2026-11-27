/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package test

import (
	"encoding/json"
	"os"
	"time"

	"github.com/LaurinStreng/terraform-provider-pbs/pbs/api"
)

// APIClient returns a low-level PBS API client for endpoints that pbs.Client
// does not wrap (certificates, namespaces, ...).
func (e *Environment) APIClient() *api.Client {
	e.t.Helper()

	c, err := api.NewClient(
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
	if err != nil {
		e.t.Fatalf("creating raw PBS API client: %v", err)
	}

	return c
}

// AddSelfRemoteVars registers template variables for a pbs_remote that points
// at the PBS under test itself, so remote scans and sync jobs work without a
// second server: {{.RemoteHost}}, {{.RemoteAuthID}}, {{.RemotePassword}},
// {{.RemoteFingerprint}}. The host defaults to localhost (as seen from PBS)
// and can be overridden with PBS_TEST_REMOTE_HOST.
func (e *Environment) AddSelfRemoteVars() {
	e.t.Helper()

	if os.Getenv("PBS_USERNAME") == "" || os.Getenv("PBS_PASSWORD") == "" {
		e.t.Skip("PBS_USERNAME/PBS_PASSWORD required for a self-referencing remote")
	}

	resp, err := e.APIClient().Get(e.Context(), "/nodes/localhost/certificates/info")
	if err != nil {
		e.t.Fatalf("reading PBS certificate info: %v", err)
	}

	var certs []struct {
		Filename    string `json:"filename"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal(resp.Data, &certs); err != nil {
		e.t.Fatalf("decoding PBS certificate info: %v", err)
	}

	fingerprint := ""
	for _, c := range certs {
		if c.Filename == "proxy.pem" {
			fingerprint = c.Fingerprint
		}
	}

	if fingerprint == "" {
		e.t.Fatalf("no proxy.pem certificate in %s", string(resp.Data))
	}

	e.AddTemplateVars(map[string]any{
		"RemoteHost":        envOr("PBS_TEST_REMOTE_HOST", "localhost"),
		"RemoteAuthID":      os.Getenv("PBS_USERNAME"),
		"RemotePassword":    os.Getenv("PBS_PASSWORD"),
		"RemoteFingerprint": fingerprint,
	})
}

// CheckLogin authenticates against the PBS under test with the given
// credentials (username/password or APIToken "user@realm!name:secret") and
// performs one authenticated request. It returns an error if PBS rejects
// the credentials, so tests can verify passwords and token secrets on the
// server instead of only in state.
func (e *Environment) CheckLogin(creds api.Credentials) error {
	e.t.Helper()

	c, err := api.NewClient(creds, api.ClientOptions{
		Endpoint: os.Getenv("PBS_ENDPOINT"),
		Insecure: os.Getenv("PBS_INSECURE") == "true",
		Timeout:  30 * time.Second,
	})
	if err != nil {
		return err
	}

	// Lists only the caller's own user without Sys.Audit, but requires a
	// valid ticket or token.
	_, err = c.Get(e.Context(), "/access/users")
	return err
}
