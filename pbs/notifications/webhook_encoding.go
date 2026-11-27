/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package notifications

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PBS stores the webhook body base64-encoded and headers and secrets as lists
// of property strings ("name=<key>,value=<base64 value>"). WebhookTarget
// exposes the decoded form; the helpers below convert between both
// representations. On read, PBS returns secrets as "name=<key>" only.

// UnmarshalJSON decodes the PBS wire format into a WebhookTarget.
func (w *WebhookTarget) UnmarshalJSON(data []byte) error {
	var wire struct {
		Name    string   `json:"name"`
		URL     string   `json:"url"`
		Body    string   `json:"body,omitempty"`
		Method  string   `json:"method,omitempty"`
		Header  []string `json:"header,omitempty"`
		Secret  []string `json:"secret,omitempty"` // "name=<key>" only, values are never returned
		Comment string   `json:"comment,omitempty"`
		Disable *bool    `json:"disable,omitempty"`
		Origin  string   `json:"origin,omitempty"`
	}

	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	body, err := decodeBase64(wire.Body)
	if err != nil {
		return fmt.Errorf("decoding webhook body: %w", err)
	}

	headers, err := decodeWebhookHeaders(wire.Header)
	if err != nil {
		return err
	}

	var secretNames []string
	for _, entry := range wire.Secret {
		name, _ := parseWebhookProperty(entry)
		secretNames = append(secretNames, name)
	}

	*w = WebhookTarget{
		Name:        wire.Name,
		URL:         wire.URL,
		Body:        body,
		Method:      wire.Method,
		Headers:     headers,
		SecretNames: secretNames,
		Comment:     wire.Comment,
		Disable:     wire.Disable,
		Origin:      wire.Origin,
	}

	return nil
}

func decodeBase64(s string) (string, error) {
	if s == "" {
		return "", nil
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func encodeWebhookBody(body string) string {
	return base64.StdEncoding.EncodeToString([]byte(body))
}

// encodeWebhookHeaders converts a header map into PBS property strings,
// sorted by name for a stable request body.
func encodeWebhookHeaders(headers map[string]string) []string {
	return encodeWebhookProperties(headers)
}

// encodeWebhookSecrets converts a secret map into PBS property strings,
// sorted by name for a stable request body. PBS uses the same encoding as for
// headers.
func encodeWebhookSecrets(secrets map[string]string) []string {
	return encodeWebhookProperties(secrets)
}

func encodeWebhookProperties(props map[string]string) []string {
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}

	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, k := range names {
		out = append(out, fmt.Sprintf("name=%s,value=%s", k, base64.StdEncoding.EncodeToString([]byte(props[k]))))
	}

	return out
}

// parseWebhookProperty splits a "name=<key>,value=<base64>" property string
// into its (still encoded) name and value parts.
func parseWebhookProperty(entry string) (name, value string) {
	for _, part := range strings.Split(entry, ",") {
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "name":
			name = v
		case "value":
			value = v
		}
	}

	return name, value
}

func decodeWebhookHeaders(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	headers := make(map[string]string, len(entries))

	for _, entry := range entries {
		name, value := parseWebhookProperty(entry)

		decoded, err := decodeBase64(value)
		if err != nil {
			return nil, fmt.Errorf("decoding webhook header %q: %w", name, err)
		}

		headers[name] = decoded
	}

	return headers, nil
}
