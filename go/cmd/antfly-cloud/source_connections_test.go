package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
			const user = "7b092f06-5621-4ea0-89dd-4e1674d12cf3"
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
					value = map[string]any{"data": []any{map[string]any{"path": "test", "uri": "test://item", "text": "message and attachment text"}}}
				case base + "/import-jobs":
					stages = append(stages, "create")
					if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
						t.Error(err)
					}
					value = map[string]any{"id": job, "name": "Test", "mode": "source", "status": "active"}
					w.WriteHeader(http.StatusCreated)
				case base + "/import-jobs/" + job + "/run":
					stages = append(stages, "sync")
					w.WriteHeader(http.StatusAccepted)
					value = map[string]any{"id": job, "status": "queued"}
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
				return &appState{cfg: Config{APIURL: server.URL + "/api/v1", Org: org, Instance: instance}, client: client, out: Output{W: &output}, timeout: time.Second}, nil
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
			source := newSourcesCommand(factory)
			source.SetArgs([]string{"add", "--connection", connection, "--resource", "gmail", "--select", "selected", "--attachments", "--name", "Test", "--table", "qualification", "--every", "24h", "--permission-ttl", "1m"})
			if err := source.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Join(stages, ",") != "authorize,connected,select,preview,create,sync" {
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
			if !strings.Contains(output.String(), "Source created and sync queued: "+job) {
				t.Fatalf("missing completion: %s", output.String())
			}
		})
	}
}
