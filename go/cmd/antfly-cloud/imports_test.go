package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/spf13/cobra"
)

const cliImportID = "459294c6-f229-4fa0-a6d2-7f5cbec020f3"
const cliImportRun = "f36960bb-e39c-496d-83ff-b127c21b1bda"

func importTestServer(t *testing.T, endpoint http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`{"data":[{"id":"9a17e518-6274-4f79-8dff-80eb53e6d86c","slug":"acme"}]}`))
		case "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances":
			_, _ = w.Write([]byte(`{"data":[{"id":"3bf7206e-c22c-47df-8126-366d4f53752d","slug":"prod"}]}`))
		default:
			endpoint(w, r)
		}
	}))
}

func importTestRoot(t *testing.T, srv *httptest.Server, out, stderr *bytes.Buffer, args ...string) *cobra.Command {
	t.Helper()
	t.Setenv("ANTFLY_CLOUD_TOKEN", "test-bearer-secret")
	cmd := newRootCommand(out, stderr)
	cmd.SetArgs(append([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "--api-url", srv.URL + "/api/v1", "--org", "acme", "--instance", "prod", "--json"}, args...))
	return cmd
}

func TestImportWaitCommandReturnsFailureAndDiagnostics(t *testing.T) {
	for _, outcome := range []string{"success", "partial", "failed", ""} {
		t.Run(outcome, func(t *testing.T) {
			srv := importTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []sdk.ImportRun{{ID: cliImportRun, Status: "complete", Diagnostics: sdk.RunDiagnostics{Outcome: outcome, Diagnostics: []sdk.RunDiagnostic{{Code: "document_extraction_failed", Source: &sdk.RunDiagnosticSource{Key: "bad.pdf"}}}}}}})
			})
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, "import", "wait", cliImportID, cliImportRun)
			err := cmd.Execute()
			if (err != nil) != (outcome != "success") {
				t.Fatalf("outcome=%q err=%v", outcome, err)
			}
			var run sdk.ImportRun
			if err := json.Unmarshal(out.Bytes(), &run); err != nil {
				t.Fatal(err)
			}
			if run.ID != cliImportRun || run.Diagnostics.Diagnostics[0].Source.Key != "bad.pdf" {
				t.Fatalf("lost failure diagnostics: %s", out.String())
			}
		})
	}
}

func TestCredentialCommandNeverPrintsErrorPayload(t *testing.T) {
	const secret = "s3-secret-never-log"
	srv := importTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"` + secret + ` test-bearer-secret"}`))
	})
	defer srv.Close()
	for _, request := range []string{`{"provider":"s3","display_name":"docs","secret":"` + secret + `","config":{}}`, `{"enabled":"` + secret + `"}`} {
		var out, stderr bytes.Buffer
		cmd := importTestRoot(t, srv, &out, &stderr, "connections", "create", "--file", "-")
		cmd.SetIn(strings.NewReader(request))
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error")
		}
		printed := out.String() + stderr.String() + err.Error()
		if strings.Contains(printed, secret) || strings.Contains(printed, "test-bearer-secret") {
			t.Fatalf("leaked credential: %s", printed)
		}
	}
}

func TestInstanceKeyCreationPrintsSecretOnlyOnce(t *testing.T) {
	const secret = "antflydb-once-only-secret"
	srv := importTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req sdk.CreateCloudAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		for _, grant := range req.Grants {
			for _, action := range grant.Actions {
				if action != "table.read" {
					http.Error(w, "invalid action", http.StatusBadRequest)
					return
				}
			}
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"f36960bb-e39c-496d-83ff-b127c21b1bda","key":"` + secret + `","key_prefix":"antflydb-once","name":"reader","key_type":"read_only"}`))
	})
	defer srv.Close()
	var out, stderr bytes.Buffer
	cmd := importTestRoot(t, srv, &out, &stderr, "instance", "api-key", "create", "--file", "-")
	cmd.SetIn(strings.NewReader(`{"name":"reader","key_type":"read_only","grants":[{"table_name":"docs","actions":["table.read"]}]}`))
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), secret) != 1 || strings.Contains(stderr.String(), secret) {
		t.Fatalf("key should be printed once on stdout; stdout=%q stderr=%q", out.String(), stderr.String())
	}
}

func TestKeyAdmissionBeforeHTTP(t *testing.T) {
	for _, input := range []string{
		`{"name":"reader","key_type":"read_only","grant":[{"table_name":"docs","actions":["table.read"]}]}`,
		`{"name":"reader","grants":[{"table":"docs","actions":["table.read"]}]}`,
		`{"grants":[{"table_name":"   "}]}`, `{"grants":[{}]}`,
		`null`, `[]`, `"secret"`, `42`, `{} {}`,
		`{"secret-never-echo":"value-never-echo"}`,
	} {
		t.Run(input, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, "instance", "api-key", "create", "--file", "-")
			cmd.SetIn(strings.NewReader(input))
			err := cmd.ExecuteContext(context.Background())
			if err == nil || calls != 0 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			if strings.Contains(err.Error()+out.String()+stderr.String(), "never-echo") {
				t.Fatal("request leaked")
			}
		})
	}
}

func TestKeyIntentionalScopes(t *testing.T) {
	for _, input := range []string{`{"grants":[{"table_name":"docs","actions":["table.read"]}]}`, `{"grants":[{"table_name":"*","actions":["table.read"]}]}`, `{}`, `{"grants":[]}`} {
		srv := importTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":"` + cliImportRun + `","key":"one-time"}`))
		})
		var out, stderr bytes.Buffer
		cmd := importTestRoot(t, srv, &out, &stderr, "instance", "api-key", "create", "--file", "-")
		cmd.SetIn(strings.NewReader(input))
		err := cmd.ExecuteContext(context.Background())
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequestObjectAdmission(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `1`, `true`, `"x"`, `{} {}`} {
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader(input))
		var raw json.RawMessage
		if err := readRequestFile(cmd, "-", &raw); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"config":{"arbitrary":{"secret":"retained"}}}`))
	var req sdk.CreateProviderAuthRequest
	if err := readRequestFile(cmd, "-", &req); err != nil {
		t.Fatal(err)
	}
	cmd.SetIn(strings.NewReader(`{"arbitrary":{"query":[1,2]}}`))
	var raw json.RawMessage
	if err := readRequestFile(cmd, "-", &raw); err != nil || !strings.Contains(string(raw), "query") {
		t.Fatalf("raw object lost: %s %v", raw, err)
	}
}

func TestTypedCreateAdmissionBeforeHTTP(t *testing.T) {
	for _, group := range []string{"connections", "import"} {
		for _, input := range []string{
			`null`, `[]`, `"secret"`, `42`, `true`, `{} {}`,
			`{"secret-never-echo":"value-never-echo"}`,
		} {
			t.Run(group+"/"+input, func(t *testing.T) {
				calls := 0
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
				defer srv.Close()
				var out, stderr bytes.Buffer
				cmd := importTestRoot(t, srv, &out, &stderr, group, "create", "--file", "-")
				cmd.SetIn(strings.NewReader(input))
				err := cmd.Execute()
				if err == nil || calls != 0 {
					t.Fatalf("typed admission: err=%v HTTP calls=%d", err, calls)
				}
				if strings.Contains(err.Error()+out.String()+stderr.String(), "never-echo") {
					t.Fatal("request leaked")
				}
			})
		}
	}
}

func TestCreateSelectorsRejectBeforeHTTP(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{"provider conflict", []string{"connections", "create", "--provider", "s3"}, `{"provider":"gcs"}`},
		{"empty provider", []string{"connections", "create", "--provider="}, `{"provider":"s3"}`},
		{"empty provider without JSON value", []string{"connections", "create", "--provider="}, `{}`},
		{"connection conflict", []string{"import", "create", "--connection", cliImportRun}, `{"provider_auth_id":"` + cliImportID + `"}`},
		{"empty connection", []string{"import", "create", "--connection="}, `{"provider_auth_id":"` + cliImportRun + `"}`},
		{"empty connection without JSON value", []string{"import", "create", "--connection="}, `{}`},
		{"table conflict", []string{"import", "create", "--table", "docs"}, `{"target_table":"other"}`},
		{"empty table", []string{"import", "create", "--table="}, `{"target_table":"docs"}`},
		{"empty table without JSON value", []string{"import", "create", "--table="}, `{}`},
		{"equal connection with conflicting table", []string{"import", "create", "--connection", cliImportRun, "--table", "docs"}, `{"provider_auth_id":"` + cliImportRun + `","target_table":"other"}`},
		{"conflicting connection with equal table", []string{"import", "create", "--connection", cliImportRun, "--table", "docs"}, `{"provider_auth_id":"` + cliImportID + `","target_table":"docs"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, append(tt.args, "--file", "-")...)
			cmd.SetIn(strings.NewReader(tt.input))
			if err := cmd.Execute(); err == nil || calls != 0 {
				t.Fatalf("selector admission: err=%v HTTP calls=%d", err, calls)
			}
		})
	}
}

func TestCreateSelectorsAcceptConsistentSources(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{"provider file only", []string{"connections", "create"}, `{"provider":"s3"}`},
		{"provider flag only", []string{"connections", "create", "--provider", "s3"}, `{}`},
		{"provider equal", []string{"connections", "create", "--provider", "s3"}, `{"provider":"s3"}`},
		{"import file only", []string{"import", "create"}, `{"provider_auth_id":"` + cliImportRun + `","target_table":"docs"}`},
		{"import flags only", []string{"import", "create", "--connection", cliImportRun, "--table", "docs"}, `{}`},
		{"import equal", []string{"import", "create", "--connection", cliImportRun, "--table", "docs"}, `{"provider_auth_id":"` + cliImportRun + `","target_table":"docs"}`},
		{"connection flag and table file", []string{"import", "create", "--connection", cliImportRun}, `{"target_table":"docs"}`},
		{"connection file and table flag", []string{"import", "create", "--table", "docs"}, `{"provider_auth_id":"` + cliImportRun + `"}`},
		{"equal connection and table file", []string{"import", "create", "--connection", cliImportRun}, `{"provider_auth_id":"` + cliImportRun + `","target_table":"docs"}`},
		{"connection file and equal table", []string{"import", "create", "--table", "docs"}, `{"provider_auth_id":"` + cliImportRun + `","target_table":"docs"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := false
			srv := importTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "creation requires POST", http.StatusMethodNotAllowed)
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/provider-auths"):
					var req sdk.CreateProviderAuthRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider != "s3" {
						http.Error(w, "unsupported provider", http.StatusBadRequest)
						return
					}
					created = true
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(sdk.ProviderAuth{ID: cliImportRun, Provider: "s3", Status: "active"})
				case strings.HasSuffix(r.URL.Path, "/imports"):
					var req sdk.CreateImportRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProviderAuthID != cliImportRun || req.TargetTable != "docs" {
						http.Error(w, "unknown connection or target table", http.StatusBadRequest)
						return
					}
					created = true
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(sdk.Import{ID: cliImportID, ProviderAuthID: cliImportRun, TableName: "docs"})
				default:
					http.NotFound(w, r)
				}
			})
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, append(tt.args, "--file", "-")...)
			cmd.SetIn(strings.NewReader(tt.input))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var result struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantID := cliImportID
			if tt.args[0] == "connections" {
				wantID = cliImportRun
			}
			if !created || result.ID != wantID {
				t.Fatalf("creation did not return the selected resource: created=%v output=%s", created, out.String())
			}
		})
	}
}

func TestObsoleteCommandsAndPositionalTablesRejectBeforeHTTP(t *testing.T) {
	for _, args := range [][]string{
		{"connection", "prod"}, {"conn", "prod"},
		{"instance", "connection", "prod"}, {"instance", "conn", "prod"},
		{"provider-auth", "list"},
		{"instance", "table", "get", "docs"}, {"instance", "table", "drop", "docs"},
		{"instance", "table", "get"}, {"instance", "table", "drop"},
		{"instance", "table", "get", "docs", "--table", "docs"},
		{"instance", "table", "drop", "docs", "--table", "docs"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, args...)
			if err := cmd.Execute(); err == nil || calls != 0 {
				t.Fatalf("invalid command reached HTTP: err=%v calls=%d", err, calls)
			}
		})
	}
}
