package sdk

import (
	"context"
	"encoding/json"
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

func TestClientBrowserAccessAndKeyCreation(t *testing.T) {
	const (
		orgID      = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instanceID = "3bf7206e-c22c-47df-8126-366d4f53752d"
	)
	requests := 0
	var createRequest CreateCloudAPIKeyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		wantPolicyPath := "/api/v1/organizations/" + orgID + "/cloud/instances/" + instanceID + "/browser-access"
		wantKeysPath := "/api/v1/organizations/" + orgID + "/cloud/instances/" + instanceID + "/api-keys"
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == wantPolicyPath:
			_, _ = w.Write([]byte(`{"enabled":true,"allowed_origins":["https://docs.example.com"],"rate_limits":{"requests_per_minute_per_key":60,"requests_per_minute_per_ip":120,"concurrent_requests_per_key":5}}`))
		case r.Method == http.MethodPut && r.URL.Path == wantPolicyPath:
			_, _ = w.Write([]byte(`{"enabled":true,"allowed_origins":["https://app.example.com"],"rate_limits":{"requests_per_minute_per_key":30,"requests_per_minute_per_ip":60,"concurrent_requests_per_key":3}}`))
		case r.Method == http.MethodPost && r.URL.Path == wantKeysPath:
			if err := json.NewDecoder(r.Body).Decode(&createRequest); err != nil {
				http.Error(w, "invalid create request", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"f36960bb-e39c-496d-83ff-b127c21b1bda","key":"antflydb_test","key_prefix":"antflydb_test","name":"Docs","key_type":"read_only","browser_access":true}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL+"/api/v1", "test-token", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	policy, err := c.BrowserAccess(ctx, orgID, instanceID)
	if err != nil || !policy.Enabled || len(policy.AllowedOrigins) != 1 {
		t.Fatalf("policy = %#v, err = %v", policy, err)
	}
	updated, err := c.UpdateBrowserAccess(ctx, orgID, instanceID, UpdateCloudBrowserAccessRequest{
		Enabled:        true,
		AllowedOrigins: []string{"https://app.example.com"},
		RateLimits: CloudBrowserRateLimits{
			RequestsPerMinutePerKey:  30,
			RequestsPerMinutePerIp:   60,
			ConcurrentRequestsPerKey: 3,
		},
	})
	if err != nil || updated.AllowedOrigins[0] != "https://app.example.com" {
		t.Fatalf("updated = %#v, err = %v", updated, err)
	}
	created, err := c.CreateCloudAPIKey(ctx, orgID, instanceID, CreateCloudAPIKeyRequest{
		Name:          "Docs",
		KeyType:       "read_only",
		BrowserAccess: true,
		Grants: []CreateCloudAPIKeyGrantRequest{{
			TableName: "cloud_acme_docs",
			Actions:   []string{"read"},
		}},
	})
	if err != nil || !created.BrowserAccess || created.Key != "antflydb_test" {
		t.Fatalf("created = %#v, err = %v", created, err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
	if len(createRequest.Grants) != 1 || createRequest.Grants[0].TableName != "cloud_acme_docs" || len(createRequest.Grants[0].Actions) != 1 || createRequest.Grants[0].Actions[0] != "read" {
		t.Fatalf("create request grants = %#v", createRequest.Grants)
	}
}
