package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A connection ID must work for callers who cannot list every connection in
// the instance (for example, writers granted access to a single table).
func TestConnectionsValidateByIDSkipsListing(t *testing.T) {
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == connectionsTestBase+"/"+connectionsTestID+"/validate":
			managementTestJSON(t, w, connectionsTestSummary("company-google", "active"))
		case r.URL.Path == connectionsTestBase:
			t.Errorf("an ID reference must not list connections")
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	if err := connectionsTestExecute(factory, &output, "validate", connectionsTestID); err != nil {
		t.Fatal(err)
	}
}

func TestConnectionNameLookupForbiddenSuggestsID(t *testing.T) {
	forbidden := func(t *testing.T) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == connectionsTestBase && r.Method == http.MethodGet {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/capabilities") {
				managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true, "configure_sources": true})
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}
	check := func(t *testing.T, err error) {
		t.Helper()
		if got := errorExitCode(err); got != 3 {
			t.Fatalf("exit = %d, want 3; err = %v", got, err)
		}
		if !strings.Contains(err.Error(), "by ID") {
			t.Fatalf("error must suggest referencing the connection by ID: %v", err)
		}
	}
	t.Run("validate", func(t *testing.T) {
		var output bytes.Buffer
		check(t, connectionsTestExecute(managementTestFactory(t, &output, false, time.Second, nil, forbidden(t)), &output, "validate", "company-google"))
	})
	t.Run("manifest", func(t *testing.T) {
		var output bytes.Buffer
		path := filepath.Join(t.TempDir(), "sources.yaml")
		manifest := "version: 1\nsources:\n  - key: docs\n    connection: company-google\n    table: knowledge\n    config: {resource: drive}\n"
		if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		check(t, managementTestExecute(managementTestFactory(t, &output, false, time.Second, nil, forbidden(t)), &output, "plan", "--file", path))
	})
}

// Shared-management requests identify the CLI like the Cloud SDK does.
func TestSourceManagementUserAgent(t *testing.T) {
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "antfly-cloud" {
			t.Errorf("User-Agent = %q, want antfly-cloud", got)
		}
		managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true})
	})
	if err := managementTestExecute(factory, &output, "capabilities"); err != nil {
		t.Fatal(err)
	}
}
