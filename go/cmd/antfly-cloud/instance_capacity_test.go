package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstanceCreateRequiresAdmittedEstimateAndPreservesShape(t *testing.T) {
	for _, scenario := range []string{"estimate", "create", "denied", "forbidden"} {
		t.Run(scenario, func(t *testing.T) {
			const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
			creates := 0
			estimates := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer management-token" {
					t.Error("missing management authentication")
				}
				switch r.URL.Path {
				case "/api/v1/organizations":
					fmt.Fprintf(w, `{"data":[{"id":%q,"slug":"showcases"}]}`, org)
				case "/api/v1/organizations/" + org + "/cloud/estimate":
					estimates++
					if scenario == "forbidden" {
						http.Error(w, `{"detail":"enterprise deployments must be enabled"}`, 403)
						return
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["operation"] != "create_instance" || body["node_config"].(map[string]any)["cpu"] != "8000m" {
						t.Errorf("wrong estimate: %v", body)
					}
					fmt.Fprintf(w, `{"allowed":%t,"monthly_commitment_cents":100,"reason":"test budget"}`, scenario != "denied")
				case "/api/v1/organizations/" + org + "/cloud/instances":
					creates++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["slug"] != "grepstein" || body["node_config"].(map[string]any)["memory"] != "32Gi" {
						t.Errorf("shape changed: %v", body)
					}
					w.WriteHeader(201)
					fmt.Fprintf(w, `{"id":"3bf7206e-c22c-47df-8126-366d4f53752d","organization_id":%q,"slug":"grepstein","name":"Grepstein","status":"pending","mode":"single","tier":"standard"}`, org)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("ANTFLY_CLOUD_TOKEN", "management-token")
			var out, errOut bytes.Buffer
			command := newRootCommand(&out, &errOut)
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--api-url", server.URL + "/api/v1", "--json", "instance", "create", "--request", "-"}
			if scenario == "estimate" {
				args = append(args, "--estimate")
			}
			command.SetArgs(args)
			command.SetIn(strings.NewReader(`{"name":"Grepstein","slug":"grepstein","tier":"standard","mode":"single","node_config":{"cpu":"8000m","memory":"32Gi","data_storage":"200Gi"}}`))
			err := command.Execute()
			if scenario == "denied" || scenario == "forbidden" {
				if err == nil {
					t.Fatal("server denial bypassed")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			want := 0
			if scenario == "create" {
				want = 1
			}
			if creates != want || estimates != 1 {
				t.Fatalf("unexpected mutations: creates=%d estimates=%d", creates, estimates)
			}
		})
	}
}

func TestCapacityRequestRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, input := range []string{`{"name":"test","enterprise_deployments_enabled":true}`, `{} {}`} {
		command := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
		command.SetIn(strings.NewReader(input))
		var request capacityRequest
		if err := readInstanceRequest(command, "-", &request); err == nil {
			t.Fatal("unsupported request fields accepted")
		}
	}
}

func TestResizeEstimateAndMutationUseResolvedInstance(t *testing.T) {
	const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	for _, estimateOnly := range []bool{true, false} {
		t.Run(fmt.Sprint(estimateOnly), func(t *testing.T) {
			mutations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/organizations":
					fmt.Fprintf(w, `{"data":[{"id":%q,"slug":"showcases"}]}`, org)
				case "/api/v1/organizations/" + org + "/cloud/instances":
					fmt.Fprintf(w, `{"data":[{"id":%q,"slug":"grepstein","mode":"single","tier":"standard"}]}`, instance)
				case "/api/v1/organizations/" + org + "/cloud/estimate":
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["instance_id"] != instance || body["operation"] != "update_instance" {
						t.Errorf("estimate targets another instance: %v", body)
					}
					fmt.Fprint(w, `{"allowed":true}`)
				case "/api/v1/organizations/" + org + "/cloud/instances/" + instance:
					mutations++
					if r.Method != "PATCH" {
						t.Errorf("wrong update method %s", r.Method)
					}
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["node_config"].(map[string]any)["data_storage"] != "500Gi" {
						t.Error("capacity changed")
					}
					fmt.Fprintf(w, `{"id":%q,"slug":"grepstein"}`, instance)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("ANTFLY_CLOUD_TOKEN", "token")
			command := newRootCommand(&bytes.Buffer{}, &bytes.Buffer{})
			args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--api-url", server.URL + "/api/v1", "instance", "resize", "grepstein", "--request", "-"}
			if estimateOnly {
				args = append(args, "--estimate")
			}
			command.SetArgs(args)
			command.SetIn(strings.NewReader(`{"node_config":{"cpu":"16000m","memory":"64Gi","data_storage":"500Gi"}}`))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			want := 1
			if estimateOnly {
				want = 0
			}
			if mutations != want {
				t.Fatal("estimate mutated capacity")
			}
		})
	}
}
