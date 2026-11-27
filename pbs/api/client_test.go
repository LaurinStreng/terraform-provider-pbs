/*
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const lockTimeoutBody = `{"data":null,"message":"Unable to acquire lock \"/etc/proxmox-backup/.datastore.lck\" - Interrupted system call (os error 4)\n"}`

// newTestClient returns a client for srv that authenticates with an API token,
// so no ticket request is made, and shortens the lock retry delays.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()

	orig := lockRetryDelays
	lockRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { lockRetryDelays = orig })

	c, err := NewClient(Credentials{APIToken: "test@pbs!tok:secret"}, ClientOptions{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	return c
}

// lockingServer answers the first failures requests with a PBS lock timeout
// and then succeeds. It records every request body to check it is resent.
func lockingServer(failures int32, calls *atomic.Int32, bodies *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)

		if bodies != nil {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			*bodies = append(*bodies, string(buf))
		}

		if n <= failures {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, lockTimeoutBody)

			return
		}

		_, _ = fmt.Fprint(w, `{"data":"ok"}`)
	}))
}

// Not parallel: the tests override the package-level lockRetryDelays.

func TestDoRequestRetriesLockTimeout(t *testing.T) {
	var calls atomic.Int32
	var bodies []string

	srv := lockingServer(2, &calls, &bodies)
	defer srv.Close()

	resp, err := newTestClient(t, srv).Put(context.Background(), "/config/datastore/x", map[string]string{"comment": "c"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}

	if string(resp.Data) != `"ok"` {
		t.Fatalf("unexpected data %s", resp.Data)
	}

	if calls.Load() != 3 {
		t.Fatalf("expected 3 requests (2 lock timeouts + success), got %d", calls.Load())
	}

	for i, b := range bodies {
		if b != bodies[0] || b == "" {
			t.Fatalf("request %d body %q differs from first body %q", i, b, bodies[0])
		}
	}
}

func TestDoRequestGivesUpAfterRetries(t *testing.T) {
	var calls atomic.Int32

	srv := lockingServer(100, &calls, nil)
	defer srv.Close()

	_, err := newTestClient(t, srv).Get(context.Background(), "/config/datastore")
	if !IsLockTimeout(err) {
		t.Fatalf("expected lock timeout error, got %v", err)
	}

	if want := int32(len(lockRetryDelays) + 1); calls.Load() != want {
		t.Fatalf("expected %d requests, got %d", want, calls.Load())
	}
}

func TestDoRequestDoesNotRetryOtherErrors(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"data":null,"message":"parameter verification failed"}`)
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).Get(context.Background(), "/config/datastore"); err == nil {
		t.Fatal("expected error")
	}

	if calls.Load() != 1 {
		t.Fatalf("expected a single request, got %d", calls.Load())
	}
}

func TestDoRequestStopsRetryingOnCancel(t *testing.T) {
	var calls atomic.Int32

	srv := lockingServer(100, &calls, nil)
	defer srv.Close()

	c := newTestClient(t, srv)
	lockRetryDelays = []time.Duration{time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for calls.Load() == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	_, err := c.Get(ctx, "/config/datastore")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if !IsLockTimeout(err) {
		t.Fatalf("expected the lock error to be preserved, got %v", err)
	}
}

func TestTaskStatusGetExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status TaskStatus
		want   int
	}{
		{"ok", TaskStatus{Status: "stopped", ExitCode: "OK"}, 0},
		{"warnings are success", TaskStatus{Status: "stopped", ExitCode: "WARNINGS: 2"}, 0},
		{"no exit status when stopped", TaskStatus{Status: "stopped"}, 0},
		{"empty string when stopped", TaskStatus{Status: "stopped", ExitCode: ""}, 0},
		{"error message", TaskStatus{Status: "stopped", ExitCode: `Unable to acquire lock "/etc/proxmox-backup/.datastore.lck" - Interrupted system call (os error 4)`}, -1},
		{"message starting with a digit", TaskStatus{Status: "stopped", ExitCode: "0 chunks could not be removed"}, -1},
		{"numeric success", TaskStatus{Status: "stopped", ExitCode: float64(0)}, 0},
		{"numeric failure", TaskStatus{Status: "stopped", ExitCode: float64(2)}, 2},
		{"running", TaskStatus{Status: "running"}, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.status.GetExitCode(); got != tt.want {
				t.Fatalf("GetExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}
