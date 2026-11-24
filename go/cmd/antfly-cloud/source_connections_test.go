package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type followUpEndpointTransport struct {
	urls []string
}

func (t *followUpEndpointTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.urls = append(t.urls, r.URL.String())
	return nil, errors.New("endpoint captured without network access")
}

func TestSourceFollowUpPreservesProductionOverride(t *testing.T) {
	const devURL = "https://dev.example.invalid/api/v1"
	for _, selection := range []string{"config", "environment"} {
		t.Run(selection, func(t *testing.T) {
			t.Setenv("ANTFLY_CLOUD_API_URL", "")
			t.Setenv("ANTFLY_CLOUD_TOKEN", "management")
			configPath := filepath.Join(t.TempDir(), "cloud config's.yaml")
			configURL := devURL
			if selection == "environment" {
				configURL = defaultAPIURL
				t.Setenv("ANTFLY_CLOUD_API_URL", devURL)
			}
			if err := os.WriteFile(configPath, []byte("api_url: "+configURL+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			transport := &followUpEndpointTransport{}
			original := http.DefaultTransport
			http.DefaultTransport = transport
			t.Cleanup(func() { http.DefaultTransport = original })
			st := &appState{cfg: Config{APIURL: defaultAPIURL}, configOverride: configPath}
			for _, followUp := range []struct {
				name    string
				command string
				args    []string
			}{
				{"watch", "sources runs wait", []string{"source-id", "run-id"}},
				{"retry", "sources run", []string{"source-id", "--idempotency-key", "original-key"}},
			} {
				t.Run(followUp.name, func(t *testing.T) {
					printed := sourceFollowUpCommand(st, "org-id", "instance-id", followUp.command, followUp.args...)
					// Parse the printed command as a user shell would, then execute
					// its arguments through the real CLI configuration path.
					script := `set -- ` + strings.TrimPrefix(printed, "antfly-cloud ") + `; printf '%s\000' "$@"`
					argv, err := exec.Command("sh", "-c", script).Output()
					if err != nil {
						t.Fatal(err)
					}
					args := strings.Split(strings.TrimSuffix(string(argv), "\x00"), "\x00")
					var output bytes.Buffer
					command := newRootCommand(&output, &output)
					command.SetArgs(args)
					transport.urls = nil
					if err := command.ExecuteContext(t.Context()); err == nil {
						t.Fatal("expected transport to stop the request")
					}
					if len(transport.urls) != 1 || strings.SplitN(transport.urls[0], "?", 2)[0] != defaultAPIURL+"/organizations" {
						t.Fatalf("follow-up selected wrong endpoint: %v", transport.urls)
					}
				})
			}
		})
	}
}

func TestConnectionsConnectUsesManagedBrowserFlow(t *testing.T) {
	const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	var output bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": org, "slug": "antfly", "name": "Antfly"}}})
		case "/api/v1/organizations/" + org + "/cloud/instances":
			json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": instance, "slug": "test", "name": "Test"}}})
		case "/api/v1/organizations/" + org + "/cloud/instances/" + instance + "/provider-auths/google_workspace/install":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body["services"].([]any)) != 2 {
				t.Error("selected services missing")
			}
			json.NewEncoder(w).Encode(map[string]any{"authorization_url": "http://" + r.Host + "/api/v1/source-oauth/google_workspace/install/" + strings.Repeat("a", 64), "flow_id": strings.Repeat("a", 64)})
		default:
			http.Error(w, r.URL.Path, 404)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "management", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	factory := func(bool) (*appState, error) {
		return &appState{cfg: Config{APIURL: server.URL + "/api/v1", Org: org, Instance: instance}, client: client, out: Output{W: &output}, timeout: time.Second}, nil
	}
	command := newSourceConnectionsCommand(factory)
	command.SetOut(&output)
	command.SetArgs([]string{"connect", "google-workspace", "--services", "drive,gmail", "--no-wait"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "/source-oauth/google_workspace/install/") || !strings.Contains(output.String(), "flow_id") {
		t.Fatalf("missing browser flow: %s", output.String())
	}
	if strings.Contains(output.String(), "client_secret") || strings.Contains(output.String(), "management") {
		t.Fatal("credential leaked")
	}
}

// Exercise the user-facing command sequence against one management API fixture.
// Provider consent and ingestion are covered separately by their contract tests.
func TestSourceSetupFlow(t *testing.T) {
	for _, provider := range []string{"google_workspace", "discord", "slack"} {
		t.Run(provider, func(t *testing.T) {
			const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
			const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
			const connection = "1631a891-318b-4ebd-82f2-9360cb419365"
			const job = "74751713-c795-4403-9cdd-357625ed800c"
			const run = "c8d339f4-12c7-44b2-b636-a39b361bf326"
			const user = "7b092f06-5621-4ea0-89dd-4e1674d12cf3"
			const configOverride = "/tmp/cloud config's.yaml"
			base := "/api/v1/organizations/" + org + "/cloud/instances/" + instance
			var output bytes.Buffer
			var stages []string
			var created map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer management" {
					t.Error("missing management authentication")
				}
				value := any(nil)
				switch r.URL.Path {
				case "/api/v1/organizations":
					value = map[string]any{"data": []any{map[string]any{"id": org, "slug": "antfly", "name": "Antfly"}}}
				case "/api/v1/users/me":
					value = map[string]any{"id": user, "email": "test@example.com"}
				case "/api/v1/organizations/" + org + "/cloud/instances":
					value = map[string]any{"data": []any{map[string]any{"id": instance, "slug": "test", "name": "Test"}}}
				case base + "/provider-auths/" + provider + "/install":
					stages = append(stages, "authorize")
					host := r.Host
					value = map[string]any{"authorization_url": "http://" + host + "/api/v1/source-oauth/" + provider + "/install/" + strings.Repeat("a", 64), "flow_id": strings.Repeat("a", 64)}
				case base + "/provider-auths/" + provider + "/install/" + strings.Repeat("a", 64):
					stages = append(stages, "connected")
					value = map[string]any{"status": "connected", "provider_auth_id": connection}
				case base + "/provider-auths":
					value = map[string]any{"data": []any{map[string]any{"id": connection, "provider": provider, "display_name": "Test", "status": "active", "enabled": true, "config": map[string]any{"services": []string{"gmail"}, "guild_id": "123", "team_id": "T123"}}}}
				case base + "/provider-auths/" + connection + "/resources":
					stages = append(stages, "select")
					value = map[string]any{"data": []any{map[string]any{"id": "selected", "name": "Test content", "readable": true}}}
				case base + "/provider-auths/" + connection + "/preview-source":
					stages = append(stages, "preview")
					var config map[string]any
					if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
						t.Error(err)
					}
					if config["attachments"] != true {
						t.Error("attachments not enabled")
					}
					if provider == "discord" && config["guild_id"] != "123" {
						t.Error("guild binding lost")
					}
					if provider == "slack" && config["team_id"] != "T123" {
						t.Error("workspace binding lost")
					}
					value = map[string]any{"data": []any{map[string]any{"path": "test", "uri": "test://item", "text": "message and attachment text", "metadata": map[string]any{"parent_identity": "workspace://parent", "related_resources": []any{map[string]any{"kind": "drive", "id": "file"}}}}}}
				case base + "/import-jobs":
					stages = append(stages, "create")
					if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
						t.Error(err)
					}
					value = map[string]any{"id": job, "name": "Test", "mode": "source", "status": "active"}
					w.WriteHeader(http.StatusCreated)
				case base + "/sources/v1/capabilities":
					value = map[string]any{"per_source_runs": true}
				case base + "/sources/v1/sources/" + job:
					value = map[string]any{"id": job, "name": "Test"}
				case base + "/sources/v1/sources/" + job + "/runs":
					stages = append(stages, "sync")
					if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") == "" {
						t.Errorf("expected a keyed run submission, got %s", r.Method)
					}
					w.WriteHeader(http.StatusAccepted)
					value = map[string]any{"id": run, "source_id": job, "state": "pending"}
				default:
					http.Error(w, r.URL.Path, 404)
					return
				}
				json.NewEncoder(w).Encode(value)
			}))
			defer server.Close()
			client, err := NewClient(server.URL+"/api/v1", "management", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			factory := func(bool) (*appState, error) {
				return &appState{cfg: Config{APIURL: server.URL + "/api/v1", Org: "antfly", Instance: "test", Token: "management"}, configOverride: configOverride, client: client, out: Output{W: &output}, timeout: time.Second}, nil
			}
			connect := newSourceConnectionsCommand(factory)
			connect.SetArgs([]string{"connect", strings.ReplaceAll(provider, "_", "-"), "--services", "gmail", "--no-browser"})
			if err := connect.Execute(); err != nil {
				t.Fatal(err)
			}
			resources := newSourceConnectionsCommand(factory)
			resources.SetArgs([]string{"resources", connection, "--resource", "gmail"})
			if err := resources.Execute(); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			preview := newSourcesCommand(factory)
			preview.SetArgs([]string{"preview", "--connection", connection, "--resource", "gmail", "--select", "selected", "--attachments"})
			if err := preview.Execute(); err != nil {
				t.Fatal(err)
			}
			var previewOutput struct {
				Data []struct {
					Metadata map[string]any `json:"metadata"`
				} `json:"data"`
			}
			if err := json.Unmarshal(output.Bytes(), &previewOutput); err != nil {
				t.Fatal(err)
			}
			if len(previewOutput.Data) != 1 || previewOutput.Data[0].Metadata["parent_identity"] != "workspace://parent" || previewOutput.Data[0].Metadata["related_resources"] == nil {
				t.Fatalf("preview output lost metadata: %s", output.String())
			}
			source := newSourcesCommand(factory)
			source.SetArgs([]string{"add", "--connection", connection, "--resource", "gmail", "--select", "selected", "--attachments", "--name", "Test", "--table", "qualification", "--every", "24h", "--permission-ttl", "1m"})
			if err := source.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Join(stages, ",") != "authorize,connected,select,preview,preview,create,sync" {
				t.Fatalf("unexpected flow: %v", stages)
			}
			if created["mode"] != "source" || created["schedule_interval_seconds"] != float64(86400) {
				t.Fatalf("wrong source settings: %v", created)
			}
			if created["source_config"].(map[string]any)["permission_ttl_seconds"] != float64(60) {
				t.Fatalf("wrong permission lifetime: %v", created)
			}
			permissions := created["source_config"].(map[string]any)["permissions"].(map[string]any)
			readers := permissions["readers"].([]any)
			if permissions["public"] != false || len(readers) != 1 || readers[0] != "user:"+user {
				t.Fatalf("source is not private: %v", permissions)
			}
			watch := "Watch: antfly-cloud --config '/tmp/cloud config'\\''s.yaml' --api-url " + shellQuote(server.URL+"/api/v1") +
				" --org " + shellQuote(org) + " --instance " + shellQuote(instance) + " sources runs wait " + shellQuote(job) + " " + shellQuote(run)
			if !strings.Contains(output.String(), watch) {
				t.Fatalf("missing scoped completion command %q: %s", watch, output.String())
			}
		})
	}
}
func TestSourceAddRunFailureReportsSavedSourceAndScopedRetry(t *testing.T) {
	const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	const connection = "1631a891-318b-4ebd-82f2-9360cb419365"
	const source = "74751713-c795-4403-9cdd-357625ed800c"
	const run = "c8d339f4-12c7-44b2-b636-a39b361bf326"
	const user = "7b092f06-5621-4ea0-89dd-4e1674d12cf3"
	const configOverride = "/tmp/cloud config's.yaml"
	base := "/api/v1/organizations/" + org + "/cloud/instances/" + instance
	var output bytes.Buffer
	var createCalls, runCalls int
	var idempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var value any
		switch r.URL.Path {
		case "/api/v1/organizations":
			value = map[string]any{"data": []any{map[string]any{"id": org, "slug": "org-slug"}}}
		case "/api/v1/organizations/" + org + "/cloud/instances":
			value = map[string]any{"data": []any{map[string]any{"id": instance, "slug": "instance-slug"}}}
		case base + "/provider-auths/" + connection + "/source-selection":
			value = map[string]any{"id": connection, "provider": "google_workspace", "status": "active", "enabled": true}
		case base + "/provider-auths/" + connection + "/preview-source":
			value = map[string]any{"data": []any{map[string]any{"path": "docs"}}}
		case "/api/v1/users/me":
			value = map[string]any{"id": user}
		case base + "/import-jobs":
			createCalls++
			w.WriteHeader(http.StatusCreated)
			value = map[string]any{"id": source, "name": "Docs", "mode": "source", "status": "active"}
		case base + "/sources/v1/capabilities":
			value = map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true}
		case base + "/sources/v1/sources/" + source:
			value = map[string]any{"id": source, "name": "Docs", "connection": connection, "table": "docs", "state": "configured", "config": map[string]any{}}
		case base + "/sources/v1/sources/" + source + "/runs":
			runCalls++
			if got := r.Header.Get("Idempotency-Key"); idempotencyKey == "" {
				idempotencyKey = got
			} else if got != idempotencyKey {
				t.Errorf("retry idempotency key = %q, want %q", got, idempotencyKey)
			}
			if runCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				value = map[string]any{"version": 1, "code": "run_queue_unavailable"}
			} else {
				w.WriteHeader(http.StatusAccepted)
				value = map[string]any{"id": run, "source_id": source, "state": "pending"}
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "cloud config's.yaml")
	config := "api_url: " + server.URL + "/api/v1\norg: org-slug\ninstance: instance-slug\ntoken: management\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	var errorOutput bytes.Buffer
	execute := func(args ...string) error {
		command := newRootCommand(&output, &errorOutput)
		command.SetArgs(append([]string{
			"--config", configPath,
			"--api-url", server.URL + "/api/v1",
			"--org", "org-slug",
			"--instance", "instance-slug",
			"--json",
		}, args...))
		return command.ExecuteContext(t.Context())
	}
	err := execute("sources", "add", "--connection", connection, "--resource", "drive", "--select", "folder", "--name", "Docs", "--table", "docs")
	if err == nil || createCalls != 1 || runCalls != 1 || idempotencyKey == "" {
		t.Fatalf("add result err=%v creates=%d runs=%d key=%q", err, createCalls, runCalls, idempotencyKey)
	}
	if got := errorExitCode(err); got != 1 {
		t.Fatalf("exit = %d, want 1; error: %v", got, err)
	}
	fields := managementTestErrorJSON(t, err)
	if fields["code"] != "request_failed" || fields["status"] != float64(http.StatusServiceUnavailable) ||
		fields["server_code"] != "run_queue_unavailable" || fields["source_id"] != source ||
		fields["idempotency_key"] != idempotencyKey {
		t.Fatalf("partial-success error metadata = %v", fields)
	}
	retry := "antfly-cloud --config " + shellQuote(configPath) + " --api-url " + shellQuote(server.URL+"/api/v1") +
		" --org " + shellQuote(org) + " --instance " + shellQuote(instance) + " sources run " + shellQuote(source) +
		" --idempotency-key " + shellQuote(idempotencyKey)
	if !strings.Contains(err.Error(), "`"+retry+"`") {
		t.Fatalf("missing scoped retry command %q: %v", retry, err)
	}

	output.Reset()
	if err := execute("sources", "run", source, "--idempotency-key", idempotencyKey); err != nil {
		t.Fatal(err)
	}
	if createCalls != 1 || runCalls != 2 {
		t.Fatalf("retry repeated creation: creates=%d runs=%d", createCalls, runCalls)
	}
	var receipt struct {
		IdempotencyKey string `json:"idempotency_key"`
		Run            struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.IdempotencyKey != idempotencyKey || receipt.Run.ID != run {
		t.Fatalf("retry receipt = %s (%v)", output.String(), err)
	}
}

func TestGCSConnectionAndPageSourceFlow(t *testing.T) {
	const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	const connection = "1631a891-318b-4ebd-82f2-9360cb419365"
	const job = "74751713-c795-4403-9cdd-357625ed800c"
	const run = "c8d339f4-12c7-44b2-b636-a39b361bf326"
	const user = "7b092f06-5621-4ea0-89dd-4e1674d12cf3"
	base := "/api/v1/organizations/" + org + "/cloud/instances/" + instance
	auth := map[string]any{"id": connection, "provider": "gcs", "display_name": "Pages", "status": "active", "enabled": true, "config": map[string]any{"bucket": "private-pages", "prefix": "repaired", "access_mode": "read_only"}}
	var output bytes.Buffer
	var calls []string
	var key string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var value any
		switch r.URL.Path {
		case "/api/v1/organizations":
			value = map[string]any{"data": []any{map[string]any{"id": org, "slug": "test"}}}
		case "/api/v1/users/me":
			value = map[string]any{"id": user}
		case "/api/v1/organizations/" + org + "/cloud/instances":
			value = map[string]any{"data": []any{map[string]any{"id": instance, "slug": "test"}}}
		case base + "/provider-auths":
			if r.Method != http.MethodPost {
				http.Error(w, "provider auth listing is forbidden", http.StatusForbidden)
				return
			}
			if r.Method == "POST" {
				calls = append(calls, "register")
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["secret"] != `{"type":"workload_identity"}` {
					t.Errorf("expected a keyless selector, decode error %v", err)
				}
				value = auth
			}
		case base + "/provider-auths/" + connection + "/source-selection":
			calls = append(calls, "resolve")
			if r.URL.Query().Get("target_table") != "pages" {
				t.Error("connection lookup must use destination table authorization")
			}
			value = auth
		case base + "/provider-auths/" + connection + "/preview-gcs-source":
			calls = append(calls, "preview")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["target_table"] != "pages" || body["prefix"] != "repaired/" || body["page_provenance"] != true {
				t.Errorf("invalid GCS preview: %v (%v)", body, err)
			}
			value = map[string]any{"data": []any{map[string]any{"path": "repaired/page.txt"}}, "verified_by": "cloudaf"}
		case base + "/import-jobs":
			calls = append(calls, "create")
			var body struct {
				SourceConfig struct {
					PageProvenance bool   `json:"page_provenance"`
					Publication    string `json:"publication"`
					Permissions    struct {
						Public  bool
						Readers []string
					}
				} `json:"source_config"`
			}
			err := json.NewDecoder(r.Body).Decode(&body)
			config := body.SourceConfig
			if err != nil || !config.PageProvenance || config.Publication != "text" || config.Permissions.Public || strings.Join(config.Permissions.Readers, ",") != "user:"+user {
				t.Errorf("invalid private GCS page source: %+v (%v)", body, err)
			}
			value = map[string]any{"id": job, "name": "Pages"}
		case base + "/sources/v1/capabilities":
			value = map[string]any{"per_source_runs": true}
		case base + "/sources/v1/sources/" + job:
			value = map[string]any{"id": job, "name": "Pages"}
		case base + "/sources/v1/sources/" + job + "/runs":
			calls = append(calls, "run")
			key = r.Header.Get("Idempotency-Key")
			w.WriteHeader(http.StatusAccepted)
			value = map[string]any{"id": run, "source_id": job, "state": "pending"}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "management", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	factory := func(bool) (*appState, error) {
		return &appState{cfg: Config{APIURL: server.URL + "/api/v1", Org: org, Instance: instance, Token: "management"}, client: client, out: Output{W: &output, JSON: true}, timeout: time.Second}, nil
	}
	register := newSourceConnectionsCommand(factory)
	register.SetArgs([]string{"add", "gcs", "--name", "Pages", "--bucket", "private-pages", "--prefix", "repaired/"})
	if err := register.Execute(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	add := newSourcesCommand(factory)
	add.SetArgs([]string{"add", "--connection", connection, "--name", "Pages", "--table", "pages", "--page-provenance"})
	if err := add.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "register,resolve,preview,create,run" {
		t.Fatalf("GCS setup stages: %s", got)
	}
	var receipt struct {
		ID             string `json:"id"`
		IdempotencyKey string `json:"idempotency_key"`
		Run            struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.ID != job || receipt.Run.ID != run || key == "" || receipt.IdempotencyKey != key {
		t.Fatalf("missing source or keyed run receipt: %s (%v)", output.String(), err)
	}
}
