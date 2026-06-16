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

func TestStatusCommandUsesReadOnlyEndpoints(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"data":[{"id":"3bf7206e-c22c-47df-8126-366d4f53752d","organization_id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","name":"Prod","slug":"prod","mode":"swarm","status":"ready","region":"us-east5","version_policy":"patch_auto","current_antfly_version":"v0.2.0","version_upgrade_status":"idle","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]}`))
		case "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/usage":
			_, _ = w.Write([]byte(`{"billing_cycle_start":"2026-01-01T00:00:00Z","billing_cycle_end":"2026-02-01T00:00:00Z","instances":[],"totals":{"queries":7,"cpu_core_hours":1.5,"memory_gib_hours":2,"disk_gib_hours":3,"storage_gib_hours":4,"s3_cold_gib_hours":0,"gcs_cold_gib_hours":0}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ANTFLY_CLOUD_TOKEN", "token")
	var out, errb bytes.Buffer
	cmd := newRootCommand(&out, &errb)
	cmd.SetArgs([]string{"--api-url", srv.URL + "/api/v1", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Prod") || !strings.Contains(out.String(), "patch_auto") || !strings.Contains(out.String(), "v0.2.0") || !strings.Contains(out.String(), "queries=7") {
		t.Fatalf("output: %s", out.String())
	}
	joined := strings.Join(seen, ",")
	for _, want := range []string{"GET /api/v1/users/me", "GET /api/v1/organizations", "GET /api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances", "GET /api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/usage"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
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
	cmd.SetArgs([]string{"--config", configPath, "--api-url", srv.URL + "/api/v1", "login", "--token", "cloudaf_abcd1234_secret"})
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
