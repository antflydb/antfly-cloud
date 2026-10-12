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

const cliImportJob = "459294c6-f229-4fa0-a6d2-7f5cbec020f3"
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
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []sdk.ImportJobRun{{ID: cliImportRun, Status: "complete", Diagnostics: sdk.RunDiagnostics{Outcome: outcome, Diagnostics: []sdk.RunDiagnostic{{Code: "document_extraction_failed", Source: &sdk.RunDiagnosticSource{Key: "bad.pdf"}}}}}}})
			})
			defer srv.Close()
			var out, stderr bytes.Buffer
			cmd := importTestRoot(t, srv, &out, &stderr, "import", "wait", cliImportJob, cliImportRun)
			err := cmd.Execute()
			if (err != nil) != (outcome != "success") {
				t.Fatalf("outcome=%q err=%v", outcome, err)
			}
			var run sdk.ImportJobRun
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
		cmd := importTestRoot(t, srv, &out, &stderr, "provider-auth", "create", "--file", "-")
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
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"f36960bb-e39c-496d-83ff-b127c21b1bda","key":"` + secret + `","key_prefix":"antflydb-once","name":"reader","key_type":"read_only"}`))
	})
	defer srv.Close()
	var out, stderr bytes.Buffer
	cmd := importTestRoot(t, srv, &out, &stderr, "instance", "api-key", "create", "--file", "-")
	cmd.SetIn(strings.NewReader(`{"name":"reader","key_type":"read_only","grants":[{"table_name":"docs","actions":["table:read"]}]}`))
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), secret) != 1 || strings.Contains(stderr.String(), secret) {
		t.Fatalf("key should be printed once on stdout; stdout=%q stderr=%q", out.String(), stderr.String())
	}
}
