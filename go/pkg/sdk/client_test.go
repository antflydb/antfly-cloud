package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInstancesReturnEveryPageAndPreserveContract(t *testing.T) {
	const (
		total    = 45
		pageSize = 17
	)
	fixtures := make([]map[string]any, total)
	for i := range fixtures {
		fixtures[i] = instanceFixture(i)
	}
	fixtures[0]["shared_inference_enabled"] = true
	fixtures[0]["storage"] = storageFixture("instance")
	fixtures[0]["node_config"] = map[string]any{
		"accelerator":              map[string]any{"count": 2, "type": "nvidia-a100"},
		"deployment_profile":       "standard",
		"engine_storage":           storageFixture("node"),
		"hot_standby":              map[string]any{"applied_lsn": 88, "catalog_prepared": true, "phase": "ready", "topology_id": "topology-1"},
		"initial_runtime":          map[string]any{"image": "us-docker.pkg.dev/antfly/runtime", "image_digest": "sha256:abc", "version": "v0.3.0"},
		"lite_import_operation_id": "import-1",
		"lite_runtime_image":       "lite-image",
		"metadata_nodes":           3,
		"data_nodes":               5,
		"cpu":                      "2",
		"memory":                   "8Gi",
		"storage":                  "10Gi",
		"metadata_storage":         "4Gi",
		"data_storage":             "20Gi",
		"replication_factor":       3,
		"shared_inference_mode":    "disabled",
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := requests * pageSize
		assertQueryInt(t, r, "offset", offset)
		end := min(offset+pageSize, total)
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": fixtures[offset:end],
			"meta": map[string]any{"offset": offset, "limit": pageSize, "total": total},
		})
	}))
	defer srv.Close()

	instances, err := mustTestClient(t, srv).Instances(context.Background(), testOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(instances) != total {
		t.Fatalf("requests = %d, instances = %d", requests, len(instances))
	}
	if instances[0].ID != testUUID(0) || instances[total-1].ID != testUUID(total-1) {
		t.Fatalf("instance boundaries = %q ... %q", instances[0].ID, instances[total-1].ID)
	}

	got := instances[0]
	if !got.SharedInferenceEnabled || got.Storage == nil || got.Storage.Artifacts == nil || got.Storage.Artifacts.Bucket != "instance-bucket" {
		t.Fatalf("instance state = %#v", got)
	}
	node := got.NodeConfig
	if node.Accelerator.Count != 2 || node.Accelerator.Type != "nvidia-a100" ||
		node.EngineStorage == nil || node.EngineStorage.Artifacts == nil || node.EngineStorage.Artifacts.Prefix != "node-prefix" ||
		node.HotStandby == nil || node.HotStandby.AppliedLsn != 88 || node.HotStandby.Phase != "ready" ||
		node.InitialRuntime == nil || node.InitialRuntime.ImageDigest != "sha256:abc" ||
		node.LiteImportOperationID != "import-1" || node.LiteRuntimeImage != "lite-image" ||
		node.MetadataNodes != 3 || node.DataNodes != 5 || node.CPU != "2" || node.Memory != "8Gi" ||
		node.Storage != "10Gi" || node.MetadataStorage != "4Gi" || node.DataStorage != "20Gi" ||
		node.ReplicationFactor != 3 || node.SharedInferenceMode != "disabled" {
		t.Fatalf("node config = %#v", node)
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
		wantTablesPath := wantPolicyPath + "/tables"
		wantKeysPath := "/api/v1/organizations/" + orgID + "/cloud/instances/" + instanceID + "/api-keys"
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == wantPolicyPath:
			_, _ = w.Write([]byte(`{"enabled":true,"allowed_origins":["https://docs.example.com"],"rate_limits":{"requests_per_minute_per_key":60,"requests_per_minute_per_ip":120,"concurrent_requests_per_key":5}}`))
		case r.Method == http.MethodPut && r.URL.Path == wantPolicyPath:
			_, _ = w.Write([]byte(`{"enabled":true,"allowed_origins":["https://app.example.com"],"rate_limits":{"requests_per_minute_per_key":30,"requests_per_minute_per_ip":60,"concurrent_requests_per_key":3}}`))
		case r.Method == http.MethodGet && r.URL.Path == wantTablesPath:
			_, _ = w.Write([]byte(`{"data":[{"table_name":"acme_prod_docs","display_name":"docs","source":"managed"}]}`))
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
	tables, err := c.BrowserAccessTables(ctx, orgID, instanceID)
	if err != nil || len(tables) != 1 || tables[0].TableName != "acme_prod_docs" {
		t.Fatalf("tables = %#v, err = %v", tables, err)
	}
	created, err := c.CreateCloudAPIKey(ctx, orgID, instanceID, CreateCloudAPIKeyRequest{
		Name:          "Docs",
		KeyType:       "read_only",
		BrowserAccess: true,
		Grants: []CreateCloudAPIKeyGrantRequest{{
			TableName: "acme_prod_docs",
			Actions:   []string{"table.read"},
		}},
	})
	if err != nil || !created.BrowserAccess || created.Key != "antflydb_test" {
		t.Fatalf("created = %#v, err = %v", created, err)
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4", requests)
	}
	if len(createRequest.Grants) != 1 || createRequest.Grants[0].TableName != "acme_prod_docs" || len(createRequest.Grants[0].Actions) != 1 || createRequest.Grants[0].Actions[0] != "table.read" {
		t.Fatalf("create request grants = %#v", createRequest.Grants)
	}
}

func TestEventsReturnEveryPage(t *testing.T) {
	const (
		total    = 43
		pageSize = 12
	)
	fixtures := make([]map[string]any, total)
	for i := range fixtures {
		fixtures[i] = eventFixture(i)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := requests * pageSize
		assertQueryInt(t, r, "offset", offset)
		end := min(offset+pageSize, total)
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": fixtures[offset:end],
			"meta": map[string]any{"offset": offset, "limit": pageSize, "total": total},
		})
	}))
	defer srv.Close()

	events, err := mustTestClient(t, srv).Events(context.Background(), testOrgID, testInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 4 || len(events) != total {
		t.Fatalf("requests = %d, events = %d", requests, len(events))
	}
	if events[0].ID != testUUID(1000) || events[total-1].Message != "event 42" || events[total-1].Metadata["sequence"] != float64(42) {
		t.Fatalf("event boundaries = %#v ... %#v", events[0], events[total-1])
	}
}

func TestOrganizationMembersReturnEveryServerSizedPage(t *testing.T) {
	const (
		total    = 205
		pageSize = 73
	)
	fixtures := make([]map[string]any, total)
	for i := range fixtures {
		fixtures[i] = memberFixture(i)
	}
	offsets := []int{0, 73, 146}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := offsets[requests]
		assertQueryInt(t, r, "offset", offset)
		end := min(offset+pageSize, total)
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": fixtures[offset:end],
			"meta": map[string]any{"offset": offset, "limit": pageSize, "total": total},
		})
	}))
	defer srv.Close()

	members, err := mustTestClient(t, srv).OrganizationMembers(context.Background(), testOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(members) != total {
		t.Fatalf("requests = %d, members = %d", requests, len(members))
	}
	if members[0].ID != testUUID(2000) || members[total-1].Email != "member-204@example.com" {
		t.Fatalf("member boundaries = %#v ... %#v", members[0], members[total-1])
	}
}

func TestOrganizationsReturnEveryPage(t *testing.T) {
	const (
		total    = 5
		pageSize = 2
	)
	fixtures := make([]map[string]any, total)
	for i := range fixtures {
		fixtures[i] = organizationFixture(i)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := requests * pageSize
		assertQueryInt(t, r, "offset", offset)
		end := min(offset+pageSize, total)
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": fixtures[offset:end],
			"meta": map[string]any{"offset": offset, "limit": pageSize, "total": total},
		})
	}))
	defer srv.Close()

	organizations, err := mustTestClient(t, srv).Organizations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 3 || len(organizations) != total || organizations[total-1].Name != "Organization 4" {
		t.Fatalf("requests = %d, organizations = %#v", requests, organizations)
	}
}

func TestUsageAndConnectionPreserveCurrentContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/cloud/usage"):
			writeTestJSON(t, w, map[string]any{
				"billing_cycle_start": "2026-10-01T00:00:00Z",
				"billing_cycle_end":   "2026-11-01T00:00:00Z",
				"instances": []map[string]any{{
					"instance_id":            testInstanceID,
					"name":                   "Production",
					"queries":                11,
					"cpu_core_hours":         12.5,
					"memory_gib_hours":       13.5,
					"disk_gib_hours":         14.5,
					"disk_usage_gib_hours":   15.5,
					"objectstore_gib_hours":  16.5,
					"accelerator_core_hours": 17.5,
				}},
				"totals": map[string]any{
					"queries":                21,
					"cpu_core_hours":         22.5,
					"memory_gib_hours":       23.5,
					"disk_gib_hours":         24.5,
					"disk_usage_gib_hours":   25.5,
					"objectstore_gib_hours":  26.5,
					"accelerator_core_hours": 27.5,
				},
			})
		case strings.HasSuffix(r.URL.Path, "/connection"):
			writeTestJSON(t, w, map[string]any{
				"proxy_url":                  "https://proxy.example.com",
				"antfly_inference_proxy_url": "https://inference.example.com",
				"dashboard_url":              "https://dashboard.example.com",
				"status":                     "ready",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := mustTestClient(t, srv)

	usage, err := client.Usage(context.Background(), testOrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(usage.Instances) != 1 {
		t.Fatalf("usage instances = %#v", usage.Instances)
	}
	instance := usage.Instances[0]
	if instance.Queries != 11 || instance.CPUCoreHours != 12.5 || instance.MemoryGiBHours != 13.5 ||
		instance.DiskGiBHours != 14.5 || instance.DiskUsageGiBHours != 15.5 ||
		instance.ObjectstoreGiBHours != 16.5 || instance.AcceleratorCoreHours != 17.5 {
		t.Fatalf("instance usage = %#v", instance)
	}
	totals := usage.Totals
	if totals.Queries != 21 || totals.CPUCoreHours != 22.5 || totals.MemoryGiBHours != 23.5 ||
		totals.DiskGiBHours != 24.5 || totals.DiskUsageGiBHours != 25.5 ||
		totals.ObjectstoreGiBHours != 26.5 || totals.AcceleratorCoreHours != 27.5 {
		t.Fatalf("usage totals = %#v", totals)
	}

	connection, err := client.Connection(context.Background(), testOrgID, testInstanceID)
	if err != nil {
		t.Fatal(err)
	}
	if connection.ProxyURL != "https://proxy.example.com" ||
		connection.AntflyInferenceProxyURL != "https://inference.example.com" ||
		connection.DashboardURL != "https://dashboard.example.com" || connection.Status != "ready" {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestPaginationReturnsLaterPageErrors(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests == 0 {
			assertQueryInt(t, r, "offset", 0)
			requests++
			writeTestJSON(t, w, map[string]any{
				"data": []map[string]any{instanceFixture(0)},
				"meta": map[string]any{"offset": 0, "limit": 1, "total": 2},
			})
			return
		}
		assertQueryInt(t, r, "offset", 1)
		requests++
		http.Error(w, "later page unavailable", http.StatusBadGateway)
	}))
	defer srv.Close()

	instances, err := mustTestClient(t, srv).Instances(context.Background(), testOrgID)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadGateway || instances != nil {
		t.Fatalf("instances = %#v, err = %#v", instances, err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestPaginationRejectsPrematureEmptyPage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertQueryInt(t, r, "offset", requests)
		if requests == 0 {
			requests++
			writeTestJSON(t, w, map[string]any{
				"data": []map[string]any{eventFixture(0)},
				"meta": map[string]any{"offset": 0, "limit": 1, "total": 2},
			})
			return
		}
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": []any{},
			"meta": map[string]any{"offset": 1, "limit": 1, "total": 2},
		})
	}))
	defer srv.Close()

	events, err := mustTestClient(t, srv).Events(context.Background(), testOrgID, testInstanceID)
	if err == nil || !strings.Contains(err.Error(), "empty page") || events != nil {
		t.Fatalf("events = %#v, err = %v", events, err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestPaginationHandlesEmptyResults(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertQueryInt(t, r, "offset", 0)
		requests++
		writeTestJSON(t, w, map[string]any{
			"data": []any{},
			"meta": map[string]any{"offset": 0, "limit": 100, "total": 0},
		})
	}))
	defer srv.Close()

	members, err := mustTestClient(t, srv).OrganizationMembers(context.Background(), testOrgID)
	if err != nil || members == nil || len(members) != 0 {
		t.Fatalf("members = %#v, err = %v", members, err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

const (
	testOrgID      = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	testInstanceID = "3bf7206e-c22c-47df-8126-366d4f53752d"
)

func mustTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(server.URL+"/api/v1", "test-token", &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func assertQueryInt(t *testing.T, r *http.Request, name string, want int) {
	t.Helper()
	got := r.URL.Query().Get(name)
	if want == 0 && got == "" {
		return
	}
	if got != strconv.Itoa(want) {
		t.Errorf("%s = %q, want %d", name, got, want)
	}
}

func testUUID(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", index)
}

func storageFixture(name string) map[string]any {
	return map[string]any{
		"artifacts": map[string]any{
			"protocol": "gcs",
			"bucket":   name + "-bucket",
			"prefix":   name + "-prefix",
		},
	}
}

func instanceFixture(index int) map[string]any {
	return map[string]any{
		"id":                 testUUID(index),
		"organization_id":    testOrgID,
		"name":               fmt.Sprintf("Instance %d", index),
		"slug":               fmt.Sprintf("instance-%d", index),
		"mode":               "single",
		"status":             "ready",
		"region":             "us-east5",
		"tier":               "starter",
		"deployment_profile": "standard",
		"node_config":        map[string]any{},
		"created_at":         "2026-01-01T00:00:00Z",
		"updated_at":         "2026-01-01T00:00:00Z",
	}
}

func eventFixture(index int) map[string]any {
	return map[string]any{
		"id":                testUUID(index + 1000),
		"cloud_instance_id": testInstanceID,
		"event_type":        "provisioning_progress",
		"message":           fmt.Sprintf("event %d", index),
		"metadata":          map[string]any{"sequence": index},
		"created_at":        "2026-01-01T00:00:00Z",
	}
}

func memberFixture(index int) map[string]any {
	userID := testUUID(index + 3000)
	return map[string]any{
		"id":              testUUID(index + 2000),
		"organization_id": testOrgID,
		"user_id":         userID,
		"role":            "member",
		"status":          "active",
		"invited_at":      "2026-01-01T00:00:00Z",
		"user": map[string]any{
			"id":           userID,
			"email":        fmt.Sprintf("member-%d@example.com", index),
			"display_name": fmt.Sprintf("Member %d", index),
			"status":       "active",
			"created_at":   "2026-01-01T00:00:00Z",
			"updated_at":   "2026-01-01T00:00:00Z",
		},
	}
}

func organizationFixture(index int) map[string]any {
	return map[string]any{
		"id":            testUUID(index + 4000),
		"name":          fmt.Sprintf("Organization %d", index),
		"slug":          fmt.Sprintf("organization-%d", index),
		"status":        "active",
		"billing_email": fmt.Sprintf("billing-%d@example.com", index),
		"owner_id":      testUUID(index + 5000),
		"created_at":    "2026-01-01T00:00:00Z",
		"updated_at":    "2026-01-01T00:00:00Z",
	}
}
