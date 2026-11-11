package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	connectionsTestID   = "1631a891-318b-4ebd-82f2-9360cb419365"
	connectionsTestBase = "/api/v1/organizations/" + managementTestOrg + "/cloud/instances/" + managementTestInstance + "/provider-auths"
)

func connectionsTestSummary(name, status string) map[string]any {
	return map[string]any{
		"id": connectionsTestID, "provider": "google_workspace",
		"display_name": name, "status": status, "enabled": true,
	}
}

func connectionsTestExecute(factory stateFactory, output *bytes.Buffer, args ...string) error {
	command := newSourceConnectionsCommand(factory)
	command.SetOut(output)
	command.SetErr(io.Discard)
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	return command.Execute()
}

func connectionsTestRenameBody(t *testing.T, r *http.Request, name string) {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode rename body: %v", err)
		return
	}
	if len(body) != 1 || body["display_name"] != name {
		t.Errorf("rename body = %v, want only display_name=%q", body, name)
	}
}

func TestConnectionsValidateByName(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		jsonOut      bool
	}{
		{name: "human", status: "active"},
		{name: "json_unhealthy_still_succeeds", status: "needs_reauth", jsonOut: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			var lists, validations atomic.Int64
			factory := managementTestFactory(t, &output, tc.jsonOut, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == connectionsTestBase:
					lists.Add(1)
					if r.URL.Query().Get("provider") != "" {
						t.Errorf("connection lookup must not filter provider: %s", r.URL.RawQuery)
					}
					managementTestJSON(t, w, map[string]any{"data": []any{connectionsTestSummary("company-google", "active")}})
				case r.Method == http.MethodPost && r.URL.Path == connectionsTestBase+"/"+connectionsTestID+"/validate":
					validations.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || len(body) != 0 {
						t.Errorf("validate body = %q, error = %v; want no body", body, err)
					}
					managementTestJSON(t, w, connectionsTestSummary("company-google", tc.status))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			})
			if err := connectionsTestExecute(factory, &output, "validate", "company-google"); err != nil {
				t.Fatalf("connections validate must succeed even for status %q: %v", tc.status, err)
			}
			if lists.Load() != 1 || validations.Load() != 1 {
				t.Errorf("list requests = %d, validate requests = %d; want one each", lists.Load(), validations.Load())
			}
			if tc.jsonOut {
				var result struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
					Status      string `json:"status"`
				}
				if err := json.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatalf("invalid connection summary JSON: %v; output: %s", err, output.String())
				}
				if result.ID != connectionsTestID || result.DisplayName != "company-google" || result.Status != tc.status {
					t.Fatalf("connection summary = %+v", result)
				}
			} else if want := "Connection company-google (" + connectionsTestID + "): " + tc.status; !strings.Contains(output.String(), want) {
				t.Fatalf("output = %q, want %q", output.String(), want)
			}
		})
	}
}

func TestConnectionsRenameByName(t *testing.T) {
	var output bytes.Buffer
	var lists, renames atomic.Int64
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == connectionsTestBase:
			lists.Add(1)
			if r.URL.Query().Get("provider") != "" {
				t.Errorf("connection lookup must not filter provider: %s", r.URL.RawQuery)
			}
			managementTestJSON(t, w, map[string]any{"data": []any{connectionsTestSummary("company-google", "active")}})
		case r.Method == http.MethodPatch && r.URL.Path == connectionsTestBase+"/"+connectionsTestID:
			renames.Add(1)
			connectionsTestRenameBody(t, r, "eng-google")
			managementTestJSON(t, w, connectionsTestSummary("eng-google", "active"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	if err := connectionsTestExecute(factory, &output, "rename", "company-google", "eng-google"); err != nil {
		t.Fatalf("connections rename: %v", err)
	}
	if lists.Load() != 1 || renames.Load() != 1 {
		t.Errorf("list requests = %d, rename requests = %d; want one each", lists.Load(), renames.Load())
	}
	if want := "Renamed connection " + connectionsTestID + " to eng-google."; !strings.Contains(output.String(), want) {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestConnectionsConnectNameRejectsNoWait(t *testing.T) {
	var output bytes.Buffer
	var requests atomic.Int64
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		t.Errorf("invalid --name/--no-wait combination made request: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	})
	err := connectionsTestExecute(factory, &output, "connect", "google-workspace", "--name", "company-google", "--no-wait")
	if err == nil {
		t.Error("--name with --no-wait must fail")
	} else {
		if got := errorExitCode(err); got != 2 {
			t.Errorf("exit code = %d, want invalid exit 2; error: %v", got, err)
		}
		if !strings.Contains(err.Error(), "--name") || !strings.Contains(err.Error(), "--no-wait") {
			t.Errorf("error must explain conflicting flags: %v", err)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("requests = %d, want zero (no install flow)", got)
	}
}

func TestConnectionsConnectNameRenamesConnectedInstallation(t *testing.T) {
	var output bytes.Buffer
	var stages []string
	flow := strings.Repeat("a", 64)
	installPath := connectionsTestBase + "/google_workspace/install"
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == installPath:
			stages = append(stages, "install")
			managementTestJSON(t, w, map[string]any{
				"authorization_url": "http://" + r.Host + "/api/v1/source-oauth/google_workspace/install/" + flow,
				"flow_id":           flow,
			})
		case r.Method == http.MethodGet && r.URL.Path == installPath+"/"+flow:
			stages = append(stages, "connected")
			managementTestJSON(t, w, map[string]any{"status": "connected", "provider_auth_id": connectionsTestID})
		case r.Method == http.MethodPatch && r.URL.Path == connectionsTestBase+"/"+connectionsTestID:
			stages = append(stages, "rename")
			connectionsTestRenameBody(t, r, "company-google")
			managementTestJSON(t, w, connectionsTestSummary("company-google", "active"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	})
	if err := connectionsTestExecute(factory, &output, "connect", "google-workspace", "--name", "company-google", "--no-browser"); err != nil {
		t.Fatalf("connections connect --name: %v", err)
	}
	if got := strings.Join(stages, ","); got != "install,connected,rename" {
		t.Errorf("flow = %q, want install,connected,rename", got)
	}
	if !strings.Contains(output.String(), "company-google") || !strings.Contains(output.String(), connectionsTestID) {
		t.Fatalf("output must identify the named connection: %s", output.String())
	}
}

func TestConnectionHelpDisambiguation(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "connection", args: []string{"connection", "--help"}, want: []string{"not provider connections", "antfly-cloud connections"}},
		{name: "instance_connection", args: []string{"instance", "connection", "--help"}, want: []string{"not provider connections", "antfly-cloud connections"}},
		{name: "connections", args: []string{"connections", "--help"}, want: []string{"source connections", "GCS", "antfly-cloud connection"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output, errorOutput bytes.Buffer
			command := newRootCommand(&output, &errorOutput)
			command.SetArgs(tc.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("%s: %v", strings.Join(tc.args, " "), err)
			}
			for _, want := range tc.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("help must contain %q:\n%s", want, output.String())
				}
			}
		})
	}
}
