package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSendsBearerAndParsesInstances(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing auth: %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"3bf7206e-c22c-47df-8126-366d4f53752d","organization_id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","name":"Prod","slug":"prod","mode":"swarm","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","target_antfly_version":"v0.2.1","version_upgrade_status":"rolling","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]}`))
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL+"/api/v1", "test-token", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := c.Instances(context.Background(), "9a17e518-6274-4f79-8dff-80eb53e6d86c")
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].Slug != "prod" || instances[0].Status != "ready" || instances[0].VersionPolicy != "patch_auto" || instances[0].TargetAntflyVersion != "v0.2.1" || instances[0].VersionUpgradeStatus != "rolling" {
		t.Fatalf("instances %#v", instances)
	}
}

func TestClientAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "nope", http.StatusForbidden) }))
	defer srv.Close()
	c, err := NewClient(srv.URL, "token", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CurrentUser(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("err = %#v", err)
	}
}
