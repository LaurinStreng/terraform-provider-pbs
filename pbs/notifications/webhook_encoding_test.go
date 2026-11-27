/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

func TestEncodeWebhookHeaders(t *testing.T) {
	t.Parallel()

	got := encodeWebhookHeaders(map[string]string{"X-Test": "1", "Content-Type": "application/json"})
	want := []string{"name=Content-Type,value=YXBwbGljYXRpb24vanNvbg==", "name=X-Test,value=MQ=="}

	if !slices.Equal(got, want) {
		t.Fatalf("encodeWebhookHeaders = %v, want %v", got, want)
	}
}

func TestEncodeWebhookSecrets(t *testing.T) {
	t.Parallel()

	got := encodeWebhookSecrets(map[string]string{"token": "secret", "other": "x"})
	want := []string{"name=other,value=eA==", "name=token,value=c2VjcmV0"}

	if !slices.Equal(got, want) {
		t.Fatalf("encodeWebhookSecrets = %v, want %v", got, want)
	}
}

func TestWebhookTargetUnmarshalJSON(t *testing.T) {
	t.Parallel()

	raw := `{
		"name": "hook",
		"url": "http://example.com",
		"method": "post",
		"body": "aGVsbG8=",
		"header": ["name=X-Test,value=MQ=="],
		"secret": ["name=token", "name=other"],
		"disable": true
	}`

	var w WebhookTarget
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if w.Body != "hello" {
		t.Errorf("Body = %q, want %q", w.Body, "hello")
	}

	if !maps.Equal(w.Headers, map[string]string{"X-Test": "1"}) {
		t.Errorf("Headers = %v", w.Headers)
	}

	if !slices.Equal(w.SecretNames, []string{"token", "other"}) {
		t.Errorf("SecretNames = %v", w.SecretNames)
	}

	if w.Name != "hook" || w.Method != "post" || w.Disable == nil || !*w.Disable {
		t.Errorf("unexpected target: %+v", w)
	}
}
