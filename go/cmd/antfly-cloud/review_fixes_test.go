package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A login refresh started while watching a run must stop when the watch's
// deadline passes, not after the HTTP client's own timeout.
func TestSourceManagementTokenRefreshObeysCancellation(t *testing.T) {
	const sourceID = "11111111-1111-4111-8111-111111111111"
	const runID = "22222222-2222-4222-8222-222222222222"
	release := make(chan struct{})
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select { // a token endpoint that never answers
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(issuer.Close)
	t.Cleanup(func() { close(release) }) // runs before issuer.Close
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		base := "/api/v1/organizations/" + managementTestOrg + "/cloud/instances/" + managementTestInstance + "/sources/v1"
		switch r.URL.Path {
		case "/api/v1/organizations":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestOrg, "slug": "antfly"}}})
		case "/api/v1/organizations/" + managementTestOrg + "/cloud/instances":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestInstance, "slug": "test"}}})
		case base + "/sources/" + sourceID:
			managementTestJSON(t, w, map[string]any{"id": sourceID, "name": "docs", "connection": "c", "table": "t", "publication": "native", "state": "configured", "config": map[string]any{}, "permissions": map[string]any{"public": false, "readers": []string{}}})
		case base + "/sources/" + sourceID + "/runs/" + runID:
			managementTestJSON(t, w, map[string]any{"id": runID, "source_id": sourceID, "state": "running", "outcome": "", "authoritative": false, "created_at": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	// Valid at startup and for the first poll; due for refresh by the second.
	expires := time.Now().Add(refreshSkew + 500*time.Millisecond).UTC().Format(time.RFC3339Nano)
	config := fmt.Sprintf("api_url: %s/api/v1\norg: %s\ninstance: %s\nauth:\n  type: oidc\n  issuer: %s\n  client_id: cli\n  id_token: id\n  refresh_token: refresh\n  expires_at: %s\n",
		api.URL, managementTestOrg, managementTestInstance, issuer.URL, expires)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"--config", path, "--timeout", "30s", "sources", "runs", "wait", sourceID, runID, "--wait-timeout", "3s"})
	start := time.Now()
	err := cmd.ExecuteContext(context.Background())
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("watch returned after %s; the stalled refresh ignored the deadline (err %v)", elapsed, err)
	}
	if got := errorExitCode(err); got != 7 {
		t.Fatalf("exit = %d, want 7 (timeout); err = %v", got, err)
	}
}

// A manifest creating several private sources looks up the current user once.
func TestSourcesManifestLooksUpCurrentUserOnce(t *testing.T) {
	var lookups atomic.Int64
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/users/me":
			lookups.Add(1)
			managementTestJSON(t, w, map[string]any{"id": "7b092f06-5621-4ea0-89dd-4e1674d12cf3"})
		case strings.HasSuffix(r.URL.Path, "/sources/v1/capabilities"):
			managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true, "configure_sources": true, "revision_preconditions": true})
		case strings.Contains(r.URL.Path, "/sources/v1/sources/") && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
			managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	var manifest strings.Builder
	manifest.WriteString("version: 1\nsources:\n")
	for _, key := range []string{"a", "b", "c"} {
		fmt.Fprintf(&manifest, "  - key: %s\n    connection: %s\n    table: docs\n    config: {resource: drive}\n", key, connectionsTestID)
	}
	path := filepath.Join(t.TempDir(), "sources.yaml")
	if err := os.WriteFile(path, []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := managementTestExecute(factory, &output, "plan", "--file", path); err != nil {
		t.Fatal(err)
	}
	if got := lookups.Load(); got != 1 {
		t.Fatalf("current user looked up %d times, want 1", got)
	}
}

// A lost submission response must not hide the key needed to retry safely.
func TestSourcesRunSubmissionErrorReportsIdempotencyKey(t *testing.T) {
	const sourceID = "11111111-1111-4111-8111-111111111111"
	var sentKey atomic.Value
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, true, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sources/v1/capabilities"):
			managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true})
		case strings.HasSuffix(r.URL.Path, "/sources/v1/sources/"+sourceID) && r.Method == http.MethodGet:
			managementTestJSON(t, w, map[string]any{"id": sourceID, "name": "docs", "connection": "c", "table": "t", "publication": "native", "state": "configured", "config": map[string]any{}, "permissions": map[string]any{"public": false, "readers": []string{}}})
		case strings.HasSuffix(r.URL.Path, "/sources/v1/sources/"+sourceID+"/runs") && r.Method == http.MethodPost:
			sentKey.Store(r.Header.Get("Idempotency-Key"))
			// The server accepted the run, but the response is lost.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	err := managementTestExecute(factory, &output, "run", sourceID)
	key, _ := sentKey.Load().(string)
	if err == nil || key == "" {
		t.Fatalf("want a submission error after sending a key; err=%v key=%q", err, key)
	}
	if !strings.Contains(err.Error(), key) {
		t.Fatalf("human error must include the idempotency key %q: %v", key, err)
	}
	fields := managementTestErrorJSON(t, err)
	if fields["idempotency_key"] != key {
		t.Fatalf("JSON error idempotency_key = %v, want %q", fields["idempotency_key"], key)
	}
}
