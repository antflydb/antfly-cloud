package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antflydb/antfly/go/pkg/sources/management"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
	"github.com/google/uuid"
)

const (
	manifestTestConnection      = "45849cf2-f636-4d8a-819a-de948455c759"
	manifestTestOtherConnection = "9839bb62-2dad-4d19-ae23-223912ee31d0"
	manifestTestUser            = "ec15fe7f-dac8-47a6-9c6e-e5ea31642518"
	manifestTestKey             = "engineering-docs"
	manifestTestYAML            = `version: 1
sources:
  - key: engineering-docs
    connection: company-google
    table: docs
    config:
      resource: drive
      folder_id: engineering
`
	manifestTestJSON = `{"version":1,"sources":[{"key":"engineering-docs","connection":"company-google","table":"docs","config":{"resource":"drive","folder_id":"engineering"}}]}`
)

type manifestTestWrite struct {
	path   string
	source management.Source
}

type manifestTestFixture struct {
	output          bytes.Buffer
	factory         stateFactory
	mu              sync.Mutex
	connections     []map[string]any
	connectionLists int
	sources         map[string]management.Source
	writes          []manifestTestWrite
}

func manifestTestConnectionEntry(id, name string, enabled bool) map[string]any {
	return map[string]any{"id": id, "provider": "google_workspace", "display_name": name, "status": "active", "enabled": enabled}
}

func newManifestTestFixture(t *testing.T) *manifestTestFixture {
	t.Helper()
	f := &manifestTestFixture{
		connections: []map[string]any{
			manifestTestConnectionEntry(manifestTestConnection, "company-google", true),
			manifestTestConnectionEntry(manifestTestOtherConnection, "company-google", false),
		},
		sources: make(map[string]management.Source),
	}
	f.factory = managementTestFactory(t, &f.output, true, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case managementTestBase + "/capabilities":
			if r.Method != http.MethodGet {
				t.Errorf("capabilities method = %s, want GET", r.Method)
			}
			managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": true, "configure_sources": true, "search_versions": []int{1, 2}})
			return
		case strings.TrimSuffix(managementTestBase, "/sources/v1") + "/provider-auths":
			if r.Method != http.MethodGet || r.URL.RawQuery != "" {
				t.Errorf("connections request = %s %s, want unfiltered GET", r.Method, r.URL)
			}
			f.connectionLists++
			managementTestJSON(t, w, map[string]any{"data": f.connections})
			return
		case "/api/v1/users/me":
			if r.Method != http.MethodGet {
				t.Errorf("current user method = %s, want GET", r.Method)
			}
			managementTestJSON(t, w, map[string]any{"id": manifestTestUser, "email": "manifest@example.com"})
			return
		}
		prefix := managementTestBase + "/sources/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("unexpected manifest request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, prefix)
		switch r.Method {
		case http.MethodGet:
			source, ok := f.sources[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
				return
			}
			managementTestJSON(t, w, source)
		case http.MethodPut:
			var source management.Source
			if err := json.NewDecoder(r.Body).Decode(&source); err != nil {
				t.Errorf("decode source PUT: %v", err)
				http.Error(w, "invalid source", http.StatusBadRequest)
				return
			}
			if source.Id != id {
				t.Errorf("source body id = %q, URL id = %q", source.Id, id)
			}
			f.writes = append(f.writes, manifestTestWrite{path: r.URL.Path, source: source})
			f.sources[id] = source
			managementTestJSON(t, w, source)
		default:
			t.Errorf("unexpected source method: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return f
}

func (f *manifestTestFixture) execute(jsonOut bool, stdin string, args ...string) (string, error) {
	f.output.Reset()
	command := newSourcesCommand(func(requireToken bool) (*appState, error) {
		st, err := f.factory(requireToken)
		if err == nil {
			st.out.JSON = jsonOut
		}
		return st, err
	})
	command.SetOut(&f.output)
	command.SetErr(io.Discard)
	command.SetIn(strings.NewReader(stdin))
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs(args)
	err := command.Execute()
	return f.output.String(), err
}

func manifestTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func manifestTestExpectedID(key string) string {
	return uuid.NewSHA1(uuid.MustParse(managementTestInstance), []byte("antfly-cloud/sources/"+key)).String()
}

func manifestTestAssertWrites(t *testing.T, f *manifestTestFixture, want int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != want {
		t.Fatalf("PUT count = %d, want %d; writes = %+v", len(f.writes), want, f.writes)
	}
}

func TestSourceManifestID(t *testing.T) {
	for _, tc := range []struct{ instance, key string }{
		{managementTestInstance, manifestTestKey},
		{managementTestInstance, "support-docs"},
		{"7a6ce329-0dcf-4377-abdc-dd8172bebd55", manifestTestKey},
	} {
		t.Run(tc.instance+"/"+tc.key, func(t *testing.T) {
			got, err := sourceManifestID(tc.instance, tc.key)
			if err != nil {
				t.Fatalf("sourceManifestID: %v", err)
			}
			parsed, err := uuid.Parse(got)
			if err != nil {
				t.Fatalf("identity %q is not a UUID: %v", got, err)
			}
			if parsed.Version() != 5 || parsed.Variant() != uuid.RFC4122 {
				t.Fatalf("identity = %s, want RFC 4122 UUID v5", parsed)
			}
			want := uuid.NewSHA1(uuid.MustParse(tc.instance), []byte("antfly-cloud/sources/"+tc.key)).String()
			if got != want {
				t.Fatalf("identity = %q, want %q", got, want)
			}
			again, err := sourceManifestID(tc.instance, tc.key)
			if err != nil || again != got {
				t.Fatalf("repeated identity = %q, %v; want %q", again, err, got)
			}
			if tc.instance != managementTestInstance || tc.key != manifestTestKey {
				if got == manifestTestExpectedID(manifestTestKey) {
					t.Fatalf("identity did not change with instance/key: %s", got)
				}
			}
		})
	}
	t.Run("invalid instance", func(t *testing.T) {
		if _, err := sourceManifestID("not-a-uuid", manifestTestKey); err == nil {
			t.Fatal("invalid instance UUID accepted")
		}
	})
}

func TestSourcesManifestLifecycle(t *testing.T) {
	for _, format := range []string{"yaml", "yml", "json", "stdin"} {
		t.Run(format, func(t *testing.T) {
			f := newManifestTestFixture(t)
			content := manifestTestYAML
			if format == "json" {
				content = manifestTestJSON
			}
			path, input := "-", content
			if format != "stdin" {
				path, input = manifestTestFile(t, "sources."+format, content), ""
			}
			id := manifestTestExpectedID(manifestTestKey)
			output, err := f.execute(false, input, "plan", "--file", path)
			manifestTestAssertWrites(t, f, 0)
			if err != nil {
				t.Fatalf("sources plan: %v; output: %s", err, output)
			}
			found := false
			for _, line := range strings.Split(output, "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 3 && fields[0] == "create" && fields[1] == manifestTestKey && fields[2] == id {
					found = true
				}
			}
			if !found {
				t.Fatalf("plan missing create row for key %s and id %s: %s", manifestTestKey, id, output)
			}

			for i, wantAction := range []commands.PlanAction{commands.PlanCreate, commands.PlanUnchanged} {
				output, err = f.execute(true, input, "apply", "--file", path)
				if err != nil {
					t.Fatalf("apply %d: %v; output: %s", i+1, err, output)
				}
				manifestTestAssertWrites(t, f, 1)
				var envelope struct {
					Results []commands.ApplyResult `json:"results"`
				}
				if err := json.Unmarshal([]byte(output), &envelope); err != nil {
					t.Fatalf("decode apply output: %v; output: %s", err, output)
				}
				results := envelope.Results
				if len(results) != 1 || results[0].Key != manifestTestKey || results[0].SourceID != id || results[0].Action != wantAction {
					t.Fatalf("apply %d = %+v, want %s for %s (%s)", i+1, results, wantAction, manifestTestKey, id)
				}
				f.mu.Lock()
				write := f.writes[0]
				f.mu.Unlock()
				if write.path != managementTestBase+"/sources/"+id || write.source.Id != id || write.source.Connection != manifestTestConnection || write.source.Name != manifestTestKey || write.source.Publication != "native" {
					t.Fatalf("incorrect source PUT: %+v", write)
				}
				if write.source.Permissions.Public || !reflect.DeepEqual(write.source.Permissions.Readers, []string{"user:" + manifestTestUser}) {
					t.Fatalf("source permissions = %+v, want private to current user", write.source.Permissions)
				}
				if write.source.Table != "docs" || !reflect.DeepEqual(write.source.Config, map[string]any{"resource": "drive", "folder_id": "engineering"}) {
					t.Fatalf("source table/config = %q/%+v", write.source.Table, write.source.Config)
				}
			}

			renamed := strings.Replace(content, "    connection:", "    name: Engineering knowledge\n    connection:", 1)
			if format == "json" {
				renamed = strings.Replace(content, `"connection":`, `"name":"Engineering knowledge","connection":`, 1)
			}
			if format == "stdin" {
				input = renamed
			} else if err := os.WriteFile(path, []byte(renamed), 0o600); err != nil {
				t.Fatal(err)
			}
			output, err = f.execute(true, input, "plan", "--file", path)
			manifestTestAssertWrites(t, f, 1)
			if err != nil {
				t.Fatalf("plan rename: %v", err)
			}
			var plan commands.Plan
			if err := json.Unmarshal([]byte(output), &plan); err != nil {
				t.Fatalf("decode rename plan: %v; output: %s", err, output)
			}
			if len(plan.Items) != 1 || plan.Items[0].Key != manifestTestKey || plan.Items[0].SourceID != id || plan.Items[0].Action != commands.PlanUpdate || !reflect.DeepEqual(plan.Items[0].Changes, []string{"name"}) {
				t.Fatalf("rename plan = %+v, want only name update at %s", plan, id)
			}
			output, err = f.execute(true, input, "apply", "--file", path)
			if err != nil {
				t.Fatalf("apply rename: %v; output: %s", err, output)
			}
			manifestTestAssertWrites(t, f, 2)
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.sources) != 1 || f.writes[1].path != managementTestBase+"/sources/"+id || f.sources[id].Name != "Engineering knowledge" {
				t.Fatalf("rename created a different source: sources=%+v writes=%+v", f.sources, f.writes)
			}
		})
	}
}

func TestSourcesManifestInvalidFile(t *testing.T) {
	for _, tc := range []struct{ name, content, diagnostic string }{
		{"unknown source field", manifestTestYAML + "    tabel: typo\n", "tabel"},
		{"unknown top-level field", manifestTestYAML + "surprise: true\n", "surprise"},
		{"non-string nested key", manifestTestYAML + "      42: invalid\n", ""},
		{"malformed YAML", "version: 1\nsources: [\n", ""},
		{"over 1 MiB", manifestTestYAML + strings.Repeat(" ", 1<<20), ""},
	} {
		for _, action := range []string{"plan", "apply"} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				f := newManifestTestFixture(t)
				path := manifestTestFile(t, "sources.yaml", tc.content)
				_, err := f.execute(true, "", action, "--file", path)
				manifestTestAssertWrites(t, f, 0)
				var failure *commands.Error
				if !errors.As(err, &failure) || failure.Code != commands.CodeInvalid || errorExitCode(err) != 2 {
					t.Fatalf("invalid manifest error = %v (exit %d), want CodeInvalid/exit 2", err, errorExitCode(err))
				}
				if !strings.Contains(failure.Message, tc.diagnostic) {
					t.Fatalf("error = %q, want diagnostic %q", failure.Message, tc.diagnostic)
				}
				if tc.name == "malformed YAML" && !strings.Contains(failure.Message, path) {
					t.Fatalf("YAML parse error %q does not mention %s", failure.Message, path)
				}
			})
		}
	}
}

func TestSourcesManifestConnectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name, ref, code string
		exit            int
		ambiguous       bool
	}{
		{"ambiguous", "company-google", commands.CodeInvalid, 2, true},
		{"unknown", "missing-google", commands.CodeNotFound, 4, false},
		{"case sensitive", "Company-Google", commands.CodeNotFound, 4, false},
		{"disabled", "disabled-google", commands.CodeNotFound, 4, false},
	} {
		for _, action := range []string{"plan", "apply"} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				f := newManifestTestFixture(t)
				f.connections = append(f.connections, manifestTestConnectionEntry(manifestTestOtherConnection, "disabled-google", false))
				if tc.ambiguous {
					f.connections = append(f.connections, manifestTestConnectionEntry(manifestTestOtherConnection, "company-google", true))
				}
				path := manifestTestFile(t, "sources.yaml", strings.Replace(manifestTestYAML, "company-google", tc.ref, 1))
				_, err := f.execute(true, "", action, "--file", path)
				manifestTestAssertWrites(t, f, 0)
				var failure *commands.Error
				if !errors.As(err, &failure) || failure.Code != tc.code || errorExitCode(err) != tc.exit {
					t.Fatalf("connection error = %v (exit %d), want %s/exit %d", err, errorExitCode(err), tc.code, tc.exit)
				}
				if tc.ambiguous {
					for _, id := range []string{manifestTestConnection, manifestTestOtherConnection} {
						if !strings.Contains(failure.Message, id) {
							t.Errorf("ambiguous diagnostic %q missing %s", failure.Message, id)
						}
					}
				} else if want := fmt.Sprintf("connection %q not found; run antfly-cloud connections list", tc.ref); failure.Message != want {
					t.Fatalf("not found diagnostic = %q, want %q", failure.Message, want)
				}
			})
		}
	}
}

func TestSourcesManifestConnectionResolution(t *testing.T) {
	for _, action := range []string{"plan", "apply"} {
		for _, byID := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/uuid=%t", action, byID), func(t *testing.T) {
				f := newManifestTestFixture(t)
				content := manifestTestYAML + strings.TrimPrefix(strings.Replace(manifestTestYAML, manifestTestKey, "support-docs", 1), "version: 1\nsources:\n")
				wantLists := 1
				if byID {
					content = strings.ReplaceAll(content, "company-google", manifestTestConnection)
					wantLists = 0
					f.connections = nil
				}
				path := manifestTestFile(t, "sources.yaml", content)
				output, err := f.execute(true, "", action, "--file", path)
				if err != nil {
					t.Fatalf("%s: %v; output: %s", action, err, output)
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.connectionLists != wantLists {
					t.Errorf("connection lists = %d, want %d", f.connectionLists, wantLists)
				}
				if action == "plan" && len(f.writes) != 0 {
					t.Fatalf("plan performed PUT: %+v", f.writes)
				}
				if action == "apply" {
					if len(f.sources) != 2 {
						t.Fatalf("applied sources = %+v, want engineering and support", f.sources)
					}
					for _, key := range []string{manifestTestKey, "support-docs"} {
						if got := f.sources[manifestTestExpectedID(key)].Connection; got != manifestTestConnection {
							t.Errorf("%s connection = %q, want %q", key, got, manifestTestConnection)
						}
					}
				}
			})
		}
	}
}

func TestSourcesManifestFileRequired(t *testing.T) {
	for _, action := range []string{"plan", "apply"} {
		t.Run(action, func(t *testing.T) {
			f := newManifestTestFixture(t)
			_, err := f.execute(false, "", action)
			manifestTestAssertWrites(t, f, 0)
			if err == nil || errorExitCode(err) != 2 || !strings.Contains(err.Error(), "file") {
				t.Fatalf("missing --file error = %v (exit %d), want exit 2 mentioning file", err, errorExitCode(err))
			}
		})
	}
}

func TestSourcesManifestHelp(t *testing.T) {
	f := newManifestTestFixture(t)
	output, err := f.execute(false, "", "--help")
	if err != nil {
		t.Fatalf("sources --help: %v", err)
	}
	for _, action := range []string{"plan", "apply"} {
		found := false
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == action {
				found = true
			}
		}
		if !found {
			t.Errorf("sources help missing %s command: %s", action, output)
		}
	}
}
