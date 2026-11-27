/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package api

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"status 404", errors.New("API request failed with status 404: Path '/api2/json/x' not found."), true},
		{"no such datastore (400)", errors.New("API request failed with status 400: no such datastore 'ds1'"), true},
		{"does not exist", errors.New("endpoint 'e1' does not exist"), true},
		{"wrapped", fmt.Errorf("get: %w", errors.New("no such job")), true},
		{"server error", errors.New("API request failed with status 500: internal error"), false},
		{"permission denied", errors.New("API request failed with status 403: permission check failed"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsNotFound(tt.err); got != tt.want {
				t.Fatalf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsLockTimeout(t *testing.T) {
	t.Parallel()

	const lockTimeout = `Unable to acquire lock "/etc/proxmox-backup/.datastore.lck" - Interrupted system call (os error 4)`

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"api 400 datastore lock", errors.New("API request failed with status 400: " + lockTimeout), true},
		{"other config file", errors.New(`API request failed with status 400: Unable to acquire lock "/etc/proxmox-backup/.sync.lck" - Interrupted system call (os error 4)`), true},
		// Failed PBS tasks report the error message as exit status.
		{"task exit status wrapped", fmt.Errorf("datastore deletion task failed: %w", errors.New("task failed after 12s: "+lockTimeout)), true},
		{"permission denied", errors.New(`Unable to acquire lock "/etc/proxmox-backup/.datastore.lck" - Permission denied (os error 13)`), false},
		{"backup group busy", errors.New("unable to acquire lock on backup group directory \"/datastore/x/vm/100\" - another backup is already running"), false},
		{"network interrupted", errors.New("read tcp 10.0.0.1:443: interrupted system call"), false},
		{"not found", errors.New("no such datastore 'x'"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsLockTimeout(tt.err); got != tt.want {
				t.Fatalf("IsLockTimeout(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
