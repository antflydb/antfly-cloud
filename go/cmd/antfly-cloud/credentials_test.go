package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
)

const cliConnectionID = "d798b983-94ba-4c58-9f8e-3387268fd03e"
const cliConnectionSecret = "replacement-secret-never-print"
const cliConnectionPath = "/api/v1/organizations/9a17e518-6274-4f79-8dff-80eb53e6d86c/cloud/instances/3bf7206e-c22c-47df-8126-366d4f53752d/provider-auths/" + cliConnectionID

type connectionLifecycleFixture struct {
	mu               sync.Mutex
	auth             sdk.ProviderAuth
	secret           string
	validationStatus string
	validationHTTP   int
	calls            []string
}

func newConnectionLifecycleFixture() *connectionLifecycleFixture {
	return &connectionLifecycleFixture{
		auth: sdk.ProviderAuth{
			ID: cliConnectionID, Provider: "s3", DisplayName: "documents",
			Enabled: true, IsDefault: true, HasSecret: true, Status: "active",
		},
		secret: "old-secret-never-print", validationStatus: "active",
	}
}

func (f *connectionLifecycleFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	switch {
	case r.URL.Path == cliConnectionPath && r.Method == http.MethodGet:
	case r.URL.Path == cliConnectionPath && r.Method == http.MethodPatch:
		var req sdk.UpdateProviderAuthRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid update", http.StatusBadRequest)
			return
		}
		if req.Secret != nil && req.ReplaceSecret != nil && *req.ReplaceSecret {
			f.secret = *req.Secret
			f.auth.HasSecret = true
			f.auth.Status = "invalid"
		}
		if req.ClearSecret != nil && *req.ClearSecret {
			f.secret = ""
			f.auth.HasSecret = false
			f.auth.Status = "invalid"
		}
		if req.Enabled != nil {
			f.auth.Enabled = *req.Enabled
		}
		if req.IsDefault != nil {
			f.auth.IsDefault = *req.IsDefault
		}
		if req.DisplayName != nil {
			f.auth.DisplayName = *req.DisplayName
		}
		if req.Config != nil {
			f.auth.Config = *req.Config
		}
	case r.URL.Path == cliConnectionPath+"/validate" && r.Method == http.MethodPost:
		if f.validationHTTP != 0 {
			w.WriteHeader(f.validationHTTP)
			_ = json.NewEncoder(w).Encode(map[string]string{"detail": f.secret + " test-bearer-secret"})
			return
		}
		f.auth.Status = f.validationStatus
	default:
		http.Error(w, "unexpected credential request", http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(f.auth)
}

func (f *connectionLifecycleFixture) snapshot() (sdk.ProviderAuth, string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.auth, f.secret, append([]string(nil), f.calls...)
}

func executeConnectionCommand(t *testing.T, srv *httptest.Server, input string, args ...string) (string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	cmd := importTestRoot(t, srv, &out, &stderr, append([]string{"connections"}, args...)...)
	cmd.SetIn(strings.NewReader(input))
	err := cmd.Execute()
	printed := out.String() + stderr.String()
	if err != nil {
		printed += err.Error()
	}
	for _, secret := range []string{cliConnectionSecret, "old-secret-never-print", "test-bearer-secret", "unknown-secret-never-echo"} {
		if strings.Contains(printed, secret) {
			t.Fatalf("credential leaked: %s", printed)
		}
	}
	return out.String(), err
}

func decodeConnectionMetadata(t *testing.T, output string) sdk.ProviderAuth {
	t.Helper()
	var auth sdk.ProviderAuth
	if err := json.Unmarshal([]byte(output), &auth); err != nil {
		t.Fatalf("expected retained connection metadata, got %q: %v", output, err)
	}
	if auth.ID != cliConnectionID {
		t.Fatalf("lost connection identity: %+v", auth)
	}
	return auth
}

func TestConnectionsUpdateReplacementLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name            string
		httpStatus      int
		status          string
		disable         bool
		initialDisabled bool
		wantError       bool
	}{
		{name: "validated", status: "active"},
		{name: "validation HTTP failure", httpStatus: http.StatusBadRequest, wantError: true},
		{name: "validation nonactive", status: "invalid", wantError: true},
		{name: "explicitly disabled stays disabled", status: "active", disable: true},
		{name: "previously disabled stays disabled", status: "active", initialDisabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newConnectionLifecycleFixture()
			fixture.auth.Enabled = !tc.initialDisabled
			fixture.validationHTTP = tc.httpStatus
			fixture.validationStatus = tc.status
			srv := importTestServer(t, fixture.serveHTTP)
			defer srv.Close()
			input := `{"secret":"` + cliConnectionSecret + `","replace_secret":true`
			if tc.disable {
				input += `,"enabled":false`
			}
			input += `}`
			output, err := executeConnectionCommand(t, srv, input, "update", cliConnectionID, "--file", "-")
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v, wantError=%v", err, tc.wantError)
			}
			auth := decodeConnectionMetadata(t, output)
			wantStatus := "active"
			if tc.wantError {
				wantStatus = "invalid"
			}
			if auth.Status != wantStatus || !auth.HasSecret || auth.Enabled == (tc.disable || tc.initialDisabled) {
				t.Fatalf("incorrect replacement metadata: %+v", auth)
			}
			stored, secret, calls := fixture.snapshot()
			if secret != cliConnectionSecret || stored.Status != wantStatus || stored.Enabled == (tc.disable || tc.initialDisabled) {
				t.Fatalf("replacement state not retained: %+v", stored)
			}
			wantCalls := []string{"PATCH " + cliConnectionPath, "POST " + cliConnectionPath + "/validate"}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("replacement must be stored before validation: %v", calls)
			}
			if tc.wantError {
				output, err = executeConnectionCommand(t, srv, "", "get", cliConnectionID)
				if err != nil {
					t.Fatal(err)
				}
				if got := decodeConnectionMetadata(t, output); got.Status != "invalid" || !got.HasSecret {
					t.Fatalf("get lost failed replacement state: %+v", got)
				}
				fixture.mu.Lock()
				fixture.validationHTTP = 0
				fixture.validationStatus = "active"
				fixture.mu.Unlock()
				output, err = executeConnectionCommand(t, srv, "", "validate", cliConnectionID)
				if err != nil {
					t.Fatal(err)
				}
				if got := decodeConnectionMetadata(t, output); got.Status != "active" || !got.HasSecret {
					t.Fatalf("explicit validation did not recover retained replacement: %+v", got)
				}
			}
		})
	}
}

func TestConnectionsUpdateWithoutReplacement(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		check func(*testing.T, sdk.ProviderAuth, string)
	}{
		{name: "clear only", input: `{"clear_secret":true}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if auth.HasSecret || secret != "" || auth.Status != "invalid" {
				t.Fatalf("secret was not cleared: %+v", auth)
			}
		}},
		{name: "disable", input: `{"enabled":false}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if auth.Enabled || !auth.HasSecret || auth.Status != "active" {
				t.Fatalf("disable changed unrelated credential state: %+v", auth)
			}
		}},
		{name: "unset default", input: `{"is_default":false}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if auth.IsDefault {
				t.Fatal("explicit false did not unset the default")
			}
		}},
		{name: "empty display name", input: `{"display_name":""}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if auth.DisplayName != "" {
				t.Fatal("explicit empty display name was discarded")
			}
		}},
		{name: "extensible config", input: `{"config":{"connector_specific":{"regions":["us-west-2"],"nested":{"mode":"archive"}}}}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			want := map[string]any{"connector_specific": map[string]any{"regions": []any{"us-west-2"}, "nested": map[string]any{"mode": "archive"}}}
			if !reflect.DeepEqual(auth.Config, want) {
				t.Fatalf("connector configuration lost: %+v", auth.Config)
			}
		}},
		{name: "explicit false clear", input: `{"clear_secret":false}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if !auth.HasSecret || secret != "old-secret-never-print" || auth.Status != "active" {
				t.Fatalf("false clear changed the stored credential: %+v", auth)
			}
		}},
		{name: "explicit false replace", input: `{"replace_secret":false}`, check: func(t *testing.T, auth sdk.ProviderAuth, secret string) {
			if !auth.HasSecret || secret != "old-secret-never-print" || auth.Status != "active" {
				t.Fatalf("false replacement changed the stored credential: %+v", auth)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newConnectionLifecycleFixture()
			srv := importTestServer(t, fixture.serveHTTP)
			defer srv.Close()
			output, err := executeConnectionCommand(t, srv, tc.input, "update", cliConnectionID, "--file", "-")
			if err != nil {
				t.Fatal(err)
			}
			auth := decodeConnectionMetadata(t, output)
			stored, secret, calls := fixture.snapshot()
			tc.check(t, auth, secret)
			tc.check(t, stored, secret)
			if !reflect.DeepEqual(calls, []string{"PATCH " + cliConnectionPath}) {
				t.Fatalf("nonreplacement update must not validate: %v", calls)
			}
		})
	}
}

func TestConnectionsUpdateAdmissionBeforeHTTP(t *testing.T) {
	for _, input := range []string{
		`{}`, `null`, `[]`, `42`, `true`, `"replacement-secret-never-print"`, `{} {}`,
		`{"secret":"replacement-secret-never-print"}`,
		`{"secret":"replacement-secret-never-print","replace_secret":false}`,
		`{"secret":"","replace_secret":true}`,
		`{"secret":"  \t\n ","replace_secret":true}`,
		`{"replace_secret":true}`,
		`{"replace_secret":true,"secret":null}`,
		`{"secret":"replacement-secret-never-print","replace_secret":true,"clear_secret":true}`,
		`{"enabled":"replacement-secret-never-print"}`,
		`{"status":"active"}`,
		`{"unknown-secret-never-echo":"replacement-secret-never-print"}`,
		`{"display_name":null,"enabled":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			var calls atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "request should have been rejected locally", http.StatusInternalServerError)
			}))
			defer srv.Close()
			output, err := executeConnectionCommand(t, srv, input, "update", cliConnectionID, "--file", "-")
			if err == nil || calls.Load() != 0 {
				t.Fatalf("invalid update reached HTTP: err=%v calls=%d", err, calls.Load())
			}
			if strings.Contains(output+err.Error(), "unknown-secret-never-echo") {
				t.Fatal("unknown field name leaked")
			}
		})
	}
}

func TestConnectionsValidateRequiresActiveStatus(t *testing.T) {
	for _, status := range []string{"active", "invalid", "pending", ""} {
		t.Run("status="+status, func(t *testing.T) {
			fixture := newConnectionLifecycleFixture()
			fixture.auth.Status = "invalid"
			fixture.validationStatus = status
			srv := importTestServer(t, fixture.serveHTTP)
			defer srv.Close()
			output, err := executeConnectionCommand(t, srv, "", "validate", cliConnectionID)
			if (err != nil) != (status != "active") {
				t.Fatalf("status=%q err=%v", status, err)
			}
			if got := decodeConnectionMetadata(t, output); got.Status != status {
				t.Fatalf("validation metadata lost: %+v", got)
			}
		})
	}
	t.Run("HTTP failure is secret safe", func(t *testing.T) {
		fixture := newConnectionLifecycleFixture()
		fixture.secret = cliConnectionSecret
		fixture.validationHTTP = http.StatusForbidden
		srv := importTestServer(t, fixture.serveHTTP)
		defer srv.Close()
		if _, err := executeConnectionCommand(t, srv, "", "validate", cliConnectionID); err == nil {
			t.Fatal("validation HTTP failure must fail the command")
		}
	})
}
