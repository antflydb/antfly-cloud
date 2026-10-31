package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusCommandRendersCompleteUsageFromReadOnlyEndpoints(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/users/me":
			_, _ = w.Write([]byte(`{"id":"11111111-1111-1111-1111-111111111111","email":"dev@antfly.io","display_name":"Dev","status":"active"}`))
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","name":"Acme","slug":"acme","status":"active"}]}`))
		case "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances":
			_, _ = w.Write([]byte(`{"data":[{"id":"3bf7206e-c22c-47df-8126-366d4f53752d","organization_id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","name":"Prod","slug":"prod","mode":"swarm","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}],"meta":{"offset":0,"limit":100,"total":1}}`))
		case "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/usage":
			_, _ = w.Write([]byte(`{"billing_cycle_start":"2026-01-01T00:00:00Z","billing_cycle_end":"2026-02-01T00:00:00Z","instances":[],"totals":{"queries":7,"cpu_core_hours":1.5,"memory_gib_hours":2,"disk_gib_hours":3,"disk_usage_gib_hours":4,"objectstore_gib_hours":5,"accelerator_core_hours":6}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ANTFLY_CLOUD_TOKEN", "token")
	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "--api-url", srv.URL + "/api/v1", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"queries=7", "cpu=1.50h", "memory=2.00GiBh", "disk_provisioned=3.00GiBh", "disk_used=4.00GiBh", "objectstore=5.00GiBh", "accelerator=6.00core-h"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q: %s", want, out.String())
		}
	}
	joined := strings.Join(seen, ",")
	for _, want := range []string{"GET /api/v1/users/me", "GET /api/v1/organizations", "GET /api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances", "GET /api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/usage"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
	for _, request := range seen {
		if !strings.HasPrefix(request, http.MethodGet+" ") {
			t.Fatalf("status made non-read-only request %s", request)
		}
	}
}

func TestUsageCommandPreservesEveryUsageMetric(t *testing.T) {
	const (
		orgID  = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instID = "3bf7206e-c22c-47df-8126-366d4f53752d"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}],"meta":{"offset":0,"limit":1,"total":1}}`))
		case "/api/v1/organizations/" + orgID + "/cloud/usage":
			_, _ = w.Write([]byte(`{"billing_cycle_start":"2026-01-01T00:00:00Z","billing_cycle_end":"2026-02-01T00:00:00Z","instances":[{"instance_id":"` + instID + `","name":"Prod","queries":11,"cpu_core_hours":1.25,"memory_gib_hours":2.5,"disk_gib_hours":3.75,"disk_usage_gib_hours":4.2,"objectstore_gib_hours":5.5,"accelerator_core_hours":6.75}],"totals":{"queries":21,"cpu_core_hours":11.25,"memory_gib_hours":12.5,"disk_gib_hours":13.75,"disk_usage_gib_hours":14.2,"objectstore_gib_hours":15.5,"accelerator_core_hours":16.75}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ANTFLY_CLOUD_TOKEN", "token")
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	run := func(args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		cmd := newRootCommand(&out, &errb)
		cmd.SetArgs(append([]string{"--config", configPath, "--api-url", srv.URL + "/api/v1"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	human := run("usage")
	for _, want := range []string{
		"DISK_PROV_GIB_H", "DISK_USED_GIB_H", "OBJECTSTORE_GIB_H", "ACCEL_CORE_H",
		"disk_provisioned=13.75GiBh", "disk_used=14.20GiBh", "objectstore=15.50GiBh", "accelerator=16.75core-h",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("usage output missing %q: %s", want, human)
		}
	}

	var got CloudUsageSummary
	output := run("--json", "usage")
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("usage JSON is invalid: %v: %s", err, output)
	}
	var rawUsage struct {
		Instances []map[string]any `json:"instances"`
		Totals    map[string]any   `json:"totals"`
	}
	if err := json.Unmarshal([]byte(output), &rawUsage); err != nil {
		t.Fatalf("usage JSON shape is invalid: %v: %s", err, output)
	}
	for _, usage := range append(rawUsage.Instances, rawUsage.Totals) {
		for _, field := range []string{"queries", "cpu_core_hours", "memory_gib_hours", "disk_gib_hours", "disk_usage_gib_hours", "objectstore_gib_hours", "accelerator_core_hours"} {
			if _, ok := usage[field]; !ok {
				t.Fatalf("usage JSON missing %q: %s", field, output)
			}
		}
		for _, removed := range []string{"storage_gib_hours", "s3_cold_gib_hours", "gcs_cold_gib_hours"} {
			if _, ok := usage[removed]; ok {
				t.Fatalf("usage JSON retained %q: %s", removed, output)
			}
		}
	}
	if got.BillingCycleStart.Format("2006-01-02T15:04:05Z07:00") != "2026-01-01T00:00:00Z" ||
		got.BillingCycleEnd.Format("2006-01-02T15:04:05Z07:00") != "2026-02-01T00:00:00Z" ||
		len(got.Instances) != 1 {
		t.Fatalf("usage envelope changed: %#v", got)
	}
	instance := got.Instances[0]
	if instance.InstanceID != instID || instance.Name != "Prod" || instance.Queries != 11 ||
		instance.CPUCoreHours != 1.25 || instance.MemoryGiBHours != 2.5 ||
		instance.DiskGiBHours != 3.75 || instance.DiskUsageGiBHours != 4.2 ||
		instance.ObjectstoreGiBHours != 5.5 || instance.AcceleratorCoreHours != 6.75 {
		t.Fatalf("instance usage changed: %#v", instance)
	}
	totals := got.Totals
	if totals.Queries != 21 || totals.CPUCoreHours != 11.25 || totals.MemoryGiBHours != 12.5 ||
		totals.DiskGiBHours != 13.75 || totals.DiskUsageGiBHours != 14.2 ||
		totals.ObjectstoreGiBHours != 15.5 || totals.AcceleratorCoreHours != 16.75 {
		t.Fatalf("usage totals changed: %#v", totals)
	}

	view := (tuiModel{
		org:  Organization{Name: "Acme", Slug: "acme"},
		snap: tuiSnapshot{Usage: &got},
	}).View()
	for _, want := range []string{
		"queries=21", "cpu=11.25h", "memory=12.50GiBh", "disk_provisioned=13.75GiBh",
		"disk_used=14.20GiBh", "objectstore=15.50GiBh", "accelerator=16.75core-h",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("TUI usage missing %q: %s", want, view)
		}
	}
}

func TestInstanceCommandsResolvePaginatedTailAndPreserveResponses(t *testing.T) {
	const (
		orgID    = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		headID   = "11111111-1111-4111-8111-111111111111"
		tailID   = "3bf7206e-c22c-47df-8126-366d4f53752d"
		proxyURL = "/cloud/v1/" + tailID
		dashURL  = "/cloud/instances/" + tailID
		aiURL    = "/cloud/v1/" + tailID + "/ai/v1"
	)
	var instanceOffsets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}],"meta":{"offset":0,"limit":1,"total":1}}`))
		case "/api/v1/organizations/" + orgID + "/cloud/instances":
			offset := r.URL.Query().Get("offset")
			instanceOffsets = append(instanceOffsets, offset)
			if offset == "" || offset == "0" {
				_, _ = w.Write([]byte(`{"data":[{"id":"` + headID + `","organization_id":"` + orgID + `","name":"Head","slug":"head","mode":"single","status":"ready","region":"us-east5"}],"meta":{"offset":0,"limit":1,"total":2}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"` + tailID + `","organization_id":"` + orgID + `","name":"Tail","slug":"tail","mode":"swarm","status":"ready","region":"us-east5"}],"meta":{"offset":1,"limit":1,"total":2}}`))
		case "/api/v1/organizations/" + orgID + "/cloud/instances/" + tailID + "/metrics":
			_, _ = w.Write([]byte(`{"instance_id":"` + tailID + `","status":"ready","storage_used_bytes":5368709120,"document_count":123456789,"table_count":17,"queries_this_month":2345,"node_count":6}`))
		case "/api/v1/organizations/" + orgID + "/cloud/instances/" + tailID + "/connection":
			_, _ = w.Write([]byte(`{"proxy_url":"` + proxyURL + `","dashboard_url":"` + dashURL + `","antfly_inference_proxy_url":"` + aiURL + `","status":"ready"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ANTFLY_CLOUD_TOKEN", "token")
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	run := func(args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		cmd := newRootCommand(&out, &errb)
		cmd.SetArgs(append([]string{"--config", configPath, "--api-url", srv.URL + "/api/v1"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}

	var metrics InstanceMetrics
	output := run("--json", "instance", "metrics", "tail")
	if err := json.Unmarshal([]byte(output), &metrics); err != nil {
		t.Fatalf("metrics JSON is invalid: %v: %s", err, output)
	}
	var rawMetrics map[string]any
	if err := json.Unmarshal([]byte(output), &rawMetrics); err != nil {
		t.Fatalf("metrics JSON shape is invalid: %v: %s", err, output)
	}
	for _, field := range []string{"instance_id", "status", "storage_used_bytes", "document_count", "table_count", "queries_this_month", "node_count"} {
		if _, ok := rawMetrics[field]; !ok {
			t.Fatalf("metrics JSON missing %q: %s", field, output)
		}
	}
	if metrics.InstanceID != tailID || metrics.Status != "ready" || metrics.StorageUsedBytes != 5368709120 ||
		metrics.DocumentCount != 123456789 || metrics.TableCount != 17 ||
		metrics.QueriesThisMonth != 2345 || metrics.NodeCount != 6 {
		t.Fatalf("metrics changed: %#v", metrics)
	}
	metricsHuman := run("instance", "metrics", "tail")
	for _, want := range []string{"Status: ready", "Storage used: 5.0GiB", "Documents: 123456789", "Tables: 17", "Queries this month: 2345", "Nodes: 6"} {
		if !strings.Contains(metricsHuman, want) {
			t.Fatalf("metrics output missing %q: %s", want, metricsHuman)
		}
	}

	var connection ConnectionDetails
	output = run("--json", "connection", "tail")
	if err := json.Unmarshal([]byte(output), &connection); err != nil {
		t.Fatalf("connection JSON is invalid: %v: %s", err, output)
	}
	var rawConnection map[string]any
	if err := json.Unmarshal([]byte(output), &rawConnection); err != nil {
		t.Fatalf("connection JSON shape is invalid: %v: %s", err, output)
	}
	for _, field := range []string{"status", "proxy_url", "dashboard_url", "antfly_inference_proxy_url"} {
		if _, ok := rawConnection[field]; !ok {
			t.Fatalf("connection JSON missing %q: %s", field, output)
		}
	}
	if connection.Status != "ready" || connection.ProxyURL != proxyURL ||
		connection.DashboardURL != dashURL || connection.AntflyInferenceProxyURL != aiURL {
		t.Fatalf("connection changed: %#v", connection)
	}
	for _, args := range [][]string{{"connection", "tail"}, {"instance", "connection", "tail"}} {
		output := run(args...)
		for _, want := range []string{"Status: ready", "Proxy URL: " + proxyURL, "Dashboard URL: " + dashURL, "Antfly Inference URL: " + aiURL} {
			if !strings.Contains(output, want) {
				t.Fatalf("%v output missing %q: %s", args, want, output)
			}
		}
	}

	var sawTailPage bool
	for _, offset := range instanceOffsets {
		if offset != "" && offset != "0" {
			sawTailPage = true
			break
		}
	}
	if !sawTailPage {
		t.Fatalf("instance lookup never requested a later page: offsets=%v", instanceOffsets)
	}
}

func TestInstanceUseAndAntflyContext(t *testing.T) {
	const (
		orgID  = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instID = "3bf7206e-c22c-47df-8126-366d4f53752d"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer id-token" {
			t.Fatalf("Authorization = %q, want Bearer id-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}]}`))
		case "/api/v1/organizations/" + orgID + "/cloud/instances":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + instID + `","organization_id":"` + orgID + `","name":"Prod","slug":"prod","mode":"swarm","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]}`))
		case "/api/v1/organizations/" + orgID + "/cloud/instances/" + instID + "/connection":
			_, _ = w.Write([]byte(`{"proxy_url":"/cloud/v1/` + instID + `","dashboard_url":"/cloud/instances/` + instID + `","antfly_inference_proxy_url":"/cloud/v1/` + instID + `/ai/v1","status":"ready"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), configDirName, defaultConfigFile)
	cfg := Config{APIURL: srv.URL + "/api/v1", Auth: AuthConfig{Type: authTypeOIDC, IDToken: "id-token"}}
	if err := saveConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	var useOut, useErr bytes.Buffer
	useCmd := newRootCommand(&useOut, &useErr)
	useCmd.SetArgs([]string{"--config", configPath, "instance", "use", "prod"})
	if err := useCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Org != orgID || loaded.Instance != instID {
		t.Fatalf("loaded config = %#v", loaded)
	}

	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", configPath, "context", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got antflyContext
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal context %q: %v", out.String(), err)
	}
	if got.URL != srv.URL+"/cloud/v1/"+instID || got.Token != "id-token" || got.Org != "acme" || got.Instance != "prod" {
		t.Fatalf("context = %#v", got)
	}

	var envOut, envErr bytes.Buffer
	envCmd := newRootCommand(&envOut, &envErr)
	envCmd.SetArgs([]string{"--config", configPath, "env"})
	if err := envCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(envOut.String(), "export ANTFLY_URL='"+srv.URL+"/cloud/v1/"+instID+"'") ||
		!strings.Contains(envOut.String(), "export ANTFLY_TOKEN='id-token'") {
		t.Fatalf("env output: %s", envOut.String())
	}
}

func TestInstancePolicyCommandPatchesVersionPolicy(t *testing.T) {
	const (
		orgID  = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instID = "3bf7206e-c22c-47df-8126-366d4f53752d"
	)
	var patchBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations/"+orgID+"/cloud/instances":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + instID + `","organization_id":"` + orgID + `","name":"Prod","slug":"prod","mode":"single","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/organizations/"+orgID+"/cloud/instances/"+instID:
			if err := json.NewDecoder(r.Body).Decode(&patchBody); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"id":"` + instID + `","organization_id":"` + orgID + `","name":"Prod","slug":"prod","mode":"single","status":"ready","region":"us-east5","version_policy":"pinned","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), configDirName, defaultConfigFile)
	if err := saveConfig(configPath, Config{APIURL: srv.URL + "/api/v1", Token: "token"}); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", configPath, "instance", "policy", "prod", "pinned"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := patchBody["version_policy"]; got != "pinned" {
		t.Fatalf("patch body = %#v", patchBody)
	}
	if !strings.Contains(out.String(), "Version policy for Prod set to pinned") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestInstanceTargetCommandsPatchManualRuntimeTarget(t *testing.T) {
	const (
		orgID  = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instID = "3bf7206e-c22c-47df-8126-366d4f53752d"
	)
	var patchBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations/"+orgID+"/cloud/instances":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + instID + `","organization_id":"` + orgID + `","name":"Prod","slug":"prod","mode":"single","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/organizations/"+orgID+"/cloud/instances/"+instID:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			patchBodies = append(patchBodies, body)
			_, _ = w.Write([]byte(`{"id":"` + instID + `","organization_id":"` + orgID + `","name":"Prod","slug":"prod","mode":"single","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","target_antfly_version":"v0.2.1","target_antfly_image":"ghcr.io/antflydb/antfly:v0.2.1","version_upgrade_status":"pending","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	configPath := filepath.Join(t.TempDir(), configDirName, defaultConfigFile)
	if err := saveConfig(configPath, Config{APIURL: srv.URL + "/api/v1", Token: "token"}); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", configPath, "instance", "target", "prod", "ghcr.io/antflydb/antfly:v0.2.1", "--version", "v0.2.1", "--digest", "sha256:abc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(patchBodies) != 1 || patchBodies[0]["target_antfly_image"] != "ghcr.io/antflydb/antfly:v0.2.1" || patchBodies[0]["target_antfly_version"] != "v0.2.1" || patchBodies[0]["target_antfly_image_digest"] != "sha256:abc" {
		t.Fatalf("patch bodies = %#v", patchBodies)
	}

	out.Reset()
	errb.Reset()
	cmd = newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", configPath, "instance", "clear-target", "prod"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(patchBodies) != 2 || patchBodies[1]["clear_antfly_version_target"] != true {
		t.Fatalf("patch bodies = %#v", patchBodies)
	}
}

func TestLoginWithAntflyCloudTokenUsesParsedConfigFlag(t *testing.T) {
	var sawUsersMe bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/users/me":
			sawUsersMe = true
			http.NotFound(w, r)
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","name":"Acme","slug":"acme","status":"active"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	work := t.TempDir()
	t.Chdir(work)
	configPath := filepath.Join(t.TempDir(), configDirName, defaultConfigFile)
	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--config", configPath, "--api-url", srv.URL + "/api/v1", "login", "--token", "antfly_cloud_abcd1234_secret"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if sawUsersMe {
		t.Fatal("antfly-cloud token login should not require /users/me")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config at --config path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, configDirName, defaultConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("unexpected default config write: %v", err)
	}
	if !strings.Contains(out.String(), "Logged in with Antfly Cloud API key") {
		t.Fatalf("output: %s", out.String())
	}
}

func TestVersionCommandDoesNotRequireLogin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "antfly-cloud ") || !strings.Contains(out.String(), "commit:") {
		t.Fatalf("version output: %s", out.String())
	}
}

func TestCompletionCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"completion", "bash"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "antfly-cloud") {
		t.Fatalf("completion output: %s", out.String())
	}
}
