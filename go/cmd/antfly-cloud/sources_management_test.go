package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
)

const (
	managementTestOrg      = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	managementTestInstance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	managementTestSource   = "11111111-1111-4111-8111-111111111111"
	managementTestBase     = "/api/v1/organizations/" + managementTestOrg + "/cloud/instances/" + managementTestInstance + "/sources/v1"
)

func managementTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func managementTestFactory(t *testing.T, output *bytes.Buffer, jsonOut bool, timeout time.Duration, token func() string, handler http.HandlerFunc) stateFactory {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestOrg, "slug": "antfly", "name": "Antfly"}}})
		case "/api/v1/organizations/" + managementTestOrg + "/cloud/instances":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestInstance, "slug": "test", "name": "Test"}}})
		default:
			handler(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return func(requireToken bool) (*appState, error) {
		if !requireToken {
			t.Error("source management must require authentication")
		}
		bearer := "mgmt-token"
		if token != nil {
			bearer = token()
		}
		client, err := NewClient(server.URL+"/api/v1", bearer, server.Client())
		if err != nil {
			return nil, err
		}
		return &appState{
			cfg:    Config{APIURL: server.URL + "/api/v1", Org: managementTestOrg, Instance: managementTestInstance, Token: bearer},
			client: client, out: Output{JSON: jsonOut, W: output}, timeout: timeout,
		}, nil
	}
}

func managementTestExecute(factory stateFactory, output *bytes.Buffer, args ...string) error {
	command := newSourcesCommand(factory)
	command.SetOut(output)
	command.SetErr(io.Discard)
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	return command.Execute()
}

func TestSourceManagementEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, apiURL, org, instance, want string
	}{
		{"base", "https://cloud.example/api/v1", managementTestOrg, managementTestInstance, "https://cloud.example" + managementTestBase},
		{"trailing_slashes", "https://cloud.example/api/v1///", managementTestOrg, managementTestInstance, "https://cloud.example" + managementTestBase},
		{"escaped_segments", "https://cloud.example/api/v1/", "org/with space", "instance?#", "https://cloud.example/api/v1/organizations/org%2Fwith%20space/cloud/instances/instance%3F%23/sources/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceManagementEndpoint(tc.apiURL, tc.org, tc.instance); got != tc.want {
				t.Fatalf("endpoint = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSourcesManagementList(t *testing.T) {
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != managementTestBase+"/sources" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("limit"); got != "20" {
			t.Errorf("limit = %q, want 20", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer mgmt-token" {
			t.Errorf("authorization = %q, want management bearer token", got)
		}
		managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs", "table": "docs", "state": "active"}}})
	})
	if err := managementTestExecute(factory, &output, "list"); err != nil {
		t.Fatalf("sources list: %v", err)
	}
	if !strings.Contains(output.String(), "engineering-docs") || !strings.Contains(output.String(), "NAME") {
		t.Fatalf("missing named source table: %s", output.String())
	}
}

func TestSourcesManagementListAll(t *testing.T) {
	var output bytes.Buffer
	var pages atomic.Int64
	factory := managementTestFactory(t, &output, true, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != managementTestBase+"/sources" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Errorf("unexpected limit: %s", r.URL.RawQuery)
		}
		switch page := pages.Add(1); page {
		case 1:
			if got := r.URL.Query().Get("cursor"); got != "" {
				t.Errorf("first cursor = %q", got)
			}
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs"}}, "next_cursor": "page+2/="})
		case 2:
			if got := r.URL.Query().Get("cursor"); got != "page+2/=" {
				t.Errorf("second cursor = %q", got)
			}
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": "22222222-2222-4222-8222-222222222222", "name": "support-docs"}}})
		default:
			t.Errorf("unexpected page request %d", page)
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})
	if err := managementTestExecute(factory, &output, "list", "--all"); err != nil {
		t.Fatalf("sources list --all: %v", err)
	}
	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("invalid list JSON: %v; output: %s", err, output.String())
	}
	if pages.Load() != 2 || len(result.Data) != 2 {
		t.Fatalf("pages = %d, data = %+v", pages.Load(), result.Data)
	}
	if result.Data[0].ID != managementTestSource || result.Data[0].Name != "engineering-docs" || result.Data[1].ID != "22222222-2222-4222-8222-222222222222" || result.Data[1].Name != "support-docs" {
		t.Fatalf("incorrect merged sources: %+v", result.Data)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["next_cursor"]; ok {
		t.Fatalf("all-pages output retains next_cursor: %s", output.String())
	}
}

func TestSourcesManagementStatusByName(t *testing.T) {
	var output bytes.Buffer
	requests := make(chan string, 8)
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		switch r.URL.Path {
		case managementTestBase + "/sources/engineering-docs":
			w.WriteHeader(http.StatusNotFound)
			managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
		case managementTestBase + "/sources":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs"}}})
		case managementTestBase + "/sources/" + managementTestSource:
			managementTestJSON(t, w, map[string]any{"id": managementTestSource, "name": "engineering-docs", "table": "docs", "state": "active"})
		case managementTestBase + "/sources/" + managementTestSource + "/runs":
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Errorf("runs limit = %q, want 1", got)
			}
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": "latest-run", "source_id": managementTestSource, "state": "finished", "outcome": "succeeded"}}})
		default:
			t.Errorf("unexpected management path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	if err := managementTestExecute(factory, &output, "status", "engineering-docs"); err != nil {
		t.Fatalf("sources status by name: %v", err)
	}
	want := []string{managementTestBase + "/sources/engineering-docs", managementTestBase + "/sources", managementTestBase + "/sources/" + managementTestSource, managementTestBase + "/sources/" + managementTestSource + "/runs"}
	var got []string
	for len(requests) > 0 {
		got = append(got, <-requests)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("resolution requests = %v, want %v", got, want)
	}
	for _, text := range []string{"engineering-docs", "Acquisition:", "Search:"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("status missing %q: %s", text, output.String())
		}
	}
}

func TestSourcesManagementCapabilitiesJSON(t *testing.T) {
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, true, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != managementTestBase+"/capabilities" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true})
	})
	if err := managementTestExecute(factory, &output, "capabilities"); err != nil {
		t.Fatalf("sources capabilities: %v", err)
	}
	var result struct {
		Runtime string `json:"runtime"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("invalid capabilities JSON: %v; output: %s", err, output.String())
	}
	if result.Runtime != "colony" {
		t.Fatalf("runtime = %q, want colony", result.Runtime)
	}
}

func TestSourcesManagementRefreshesTokenPerRequest(t *testing.T) {
	var output bytes.Buffer
	var calls atomic.Int64
	headers := make(chan string, 4)
	factory := managementTestFactory(t, &output, false, time.Second, func() string {
		return fmt.Sprintf("tok-%d", calls.Add(1))
	}, func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		headers <- got
		if want := fmt.Sprintf("Bearer tok-%d", calls.Load()); got != want {
			t.Errorf("stale request token = %q, want %q", got, want)
		}
		switch r.URL.Path {
		case managementTestBase + "/sources/" + managementTestSource:
			managementTestJSON(t, w, map[string]any{"id": managementTestSource, "name": "engineering-docs"})
		case managementTestBase + "/sources/" + managementTestSource + "/runs":
			managementTestJSON(t, w, map[string]any{"data": []any{}})
		default:
			t.Errorf("unexpected management path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	if err := managementTestExecute(factory, &output, "status", managementTestSource); err != nil {
		t.Fatalf("sources status: %v", err)
	}
	if len(headers) != 2 {
		t.Fatalf("management requests = %d, want 2", len(headers))
	}
	first, second := <-headers, <-headers
	if first == second || calls.Load() < 3 {
		t.Fatalf("tokens not refreshed for each request: first=%q second=%q factory calls=%d", first, second, calls.Load())
	}
}

func managementTestErrorJSON(t *testing.T, err error) map[string]any {
	t.Helper()
	var output bytes.Buffer
	writeError(&output, true, err)
	raw := output.String()
	if !strings.HasSuffix(raw, "\n") || strings.Count(raw, "\n") != 1 {
		t.Fatalf("error must be one newline-terminated JSON line: %q", raw)
	}
	var envelope map[string]map[string]any
	if decodeErr := json.Unmarshal(output.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("invalid error JSON: %v; output: %q", decodeErr, raw)
	}
	fields, ok := envelope["error"]
	if !ok || len(envelope) != 1 {
		t.Fatalf("invalid error envelope: %s", raw)
	}
	if fields["message"] != err.Error() {
		t.Errorf("message = %v, want %q", fields["message"], err.Error())
	}
	return fields
}

func TestSourcesManagementUnauthorized(t *testing.T) {
	var output bytes.Buffer
	factory := managementTestFactory(t, &output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != managementTestBase+"/sources" {
			t.Errorf("unexpected management path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		managementTestJSON(t, w, map[string]any{"version": 1, "code": "unauthorized", "message": "Bearer mgmt-token"})
	})
	err := managementTestExecute(factory, &output, "list")
	if err == nil {
		t.Fatal("sources list succeeded despite HTTP 401")
	}
	if got := errorExitCode(err); got != 3 {
		t.Errorf("exit code = %d, want 3", got)
	}
	fields := managementTestErrorJSON(t, err)
	if fields["code"] != "unauthorized" || fields["status"] != float64(401) {
		t.Errorf("unauthorized error = %v", fields)
	}
	raw, marshalErr := json.Marshal(fields)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(raw), "mgmt-token") || strings.Contains(string(raw), "Bearer") {
		t.Fatalf("credentials leaked in error: %s", raw)
	}
}

func TestSourcesManagementTimeout(t *testing.T) {
	var output bytes.Buffer
	var reached atomic.Bool
	factory := managementTestFactory(t, &output, false, 50*time.Millisecond, nil, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != managementTestBase+"/capabilities" {
			t.Errorf("unexpected management path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		reached.Store(true)
		time.Sleep(300 * time.Millisecond)
		managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony"})
	})
	err := managementTestExecute(factory, &output, "capabilities")
	if err == nil {
		t.Fatal("capabilities succeeded despite request exceeding timeout")
	}
	if !reached.Load() {
		t.Error("capabilities request did not reach the management endpoint")
	}
	if got := errorExitCode(err); got != 7 {
		t.Fatalf("exit code = %d, want timeout exit 7; error: %v", got, err)
	}
}

func TestSourcesManagementScriptableErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		exit   int
	}{
		{401, "unauthorized", 3}, {403, "forbidden", 3}, {404, "not_found", 4},
		{409, "conflict", 5}, {412, "conflict", 5}, {400, "invalid", 2},
		{422, "invalid", 2}, {500, "request_failed", 1},
	} {
		t.Run(fmt.Sprintf("wrapped_sdk_%d", tc.status), func(t *testing.T) {
			err := fmt.Errorf("selected instance: %w", &APIError{StatusCode: tc.status})
			if got := errorExitCode(err); got != tc.exit {
				t.Errorf("exit code = %d, want %d", got, tc.exit)
			}
			fields := managementTestErrorJSON(t, err)
			if fields["code"] != tc.code || fields["status"] != float64(tc.status) {
				t.Errorf("SDK error = %v, want code=%s status=%d", fields, tc.code, tc.status)
			}
			if _, ok := fields["server_code"]; ok {
				t.Errorf("SDK error has empty server_code: %v", fields)
			}
		})
	}
	t.Run("wrapped_commands_error", func(t *testing.T) {
		err := fmt.Errorf("source lookup: %w", &commands.Error{Code: "not_found", HTTPStatus: 404, ServerCode: "source_not_found", Message: "source missing"})
		if got := errorExitCode(err); got != 4 {
			t.Errorf("exit code = %d, want 4", got)
		}
		fields := managementTestErrorJSON(t, err)
		if fields["code"] != "not_found" || fields["status"] != float64(404) || fields["server_code"] != "source_not_found" {
			t.Errorf("commands error fields = %v", fields)
		}
	})
	t.Run("plain_error", func(t *testing.T) {
		err := errors.New("boom")
		if got := errorExitCode(err); got != 1 {
			t.Errorf("exit code = %d, want 1", got)
		}
		fields := managementTestErrorJSON(t, err)
		if fields["code"] != "error" {
			t.Errorf("plain error code = %v, want error", fields["code"])
		}
		for _, key := range []string{"status", "server_code"} {
			if _, ok := fields[key]; ok {
				t.Errorf("plain error retains empty %s: %v", key, fields)
			}
		}
	})
	t.Run("legacy_human_error", func(t *testing.T) {
		var output bytes.Buffer
		writeError(&output, false, errors.New("boom"))
		if got := output.String(); got != "Error: boom\n" {
			t.Fatalf("human error = %q, want legacy format", got)
		}
	})
}

func TestSourcesManagementHelp(t *testing.T) {
	var output bytes.Buffer
	root := newRootCommand(&output, &output)
	root.SetArgs([]string{"sources", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sources help: %v", err)
	}
	for _, name := range []string{"capabilities", "list", "status"} {
		if !strings.Contains(output.String(), "  "+name+" ") {
			t.Errorf("sources help missing %q: %s", name, output.String())
		}
	}
}
