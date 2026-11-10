package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSourceInstallationAndResourceClientContract(t *testing.T) {
	org := "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	instance := "3bf7206e-c22c-47df-8126-366d4f53752d"
	auth := "631cd87b-035c-4e05-ae2e-421eae4c0f96"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer management-token" {
			t.Error("management authentication missing")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/google_workspace/install"):
			var body struct {
				Services []string `json:"services"`
				AuthID   string   `json:"auth_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if strings.Join(body.Services, ",") != "drive,gmail" || body.AuthID != auth {
				t.Errorf("unexpected request: %+v", body)
			}
			json.NewEncoder(w).Encode(SourceInstallation{AuthorizationURL: "https://accounts.google.com/o/oauth2/v2/auth?state=opaque", FlowID: strings.Repeat("a", 64)})
		case strings.HasSuffix(r.URL.Path, "/resources"):
			if r.URL.Query().Get("resource") != "gmail" {
				t.Error("resource selection missing")
			}
			json.NewEncoder(w).Encode(SourceResourcePage{Data: []SourceResource{{ID: "INBOX", Name: "Inbox", Readable: true}}})
		default:
			http.Error(w, "unexpected path", 404)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	installation, err := client.StartSourceInstallation(context.Background(), org, instance, "google_workspace", auth, []string{"drive", "gmail"})
	if err != nil || installation.FlowID == "" {
		t.Fatalf("installation: %+v %v", installation, err)
	}
	page, err := client.SourceResources(context.Background(), org, instance, auth, "gmail", "", "")
	if err != nil || len(page.Data) != 1 || !page.Data[0].Readable {
		t.Fatalf("resources: %+v %v", page, err)
	}
	if _, err := client.SourceResources(context.Background(), org, instance, "../token", "gmail", "", ""); err == nil {
		t.Fatal("invalid connection ID accepted")
	}
}

func TestSourceInstallationRejectsUntrustedRedirects(t *testing.T) {
	client, err := NewClient("https://platform.antfly.io/api/v1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	flow := strings.Repeat("a", 64)
	path := "/api/v1/source-oauth/google_workspace/install/" + flow
	for _, raw := range []string{"https://accounts.google.com/authorize", "https://evil.test" + path, "http://platform.antfly.io" + path, "https://user@platform.antfly.io" + path, "https://platform.antfly.io" + path + "?next=https://evil.test", "https://platform.antfly.io/api/v1/other"} {
		if client.ValidateSourceInstallationURL(raw, "google_workspace", flow) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if err := client.ValidateSourceInstallationURL("https://platform.antfly.io"+path, "google_workspace", flow); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePreviewPreservesRelationshipMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"path":"calendar/cal/event/attachments/file","uri":"https://drive.google.com/file/d/file/view","text":"sample","metadata":{"parent_identity":"workspace://parent","attachment_id":"file","related_resources":[{"kind":"drive","id":"file"}],"calendar_invitation":true}}]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "fixture", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	preview, err := client.PreviewSource(context.Background(), "9a17e518-6274-4f79-8dff-80eb53e6d86c", "3bf7206e-c22c-47df-8126-366d4f53752d", "631cd87b-035c-4e05-ae2e-421eae4c0f96", map[string]any{"resource": "calendar"})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Data) != 1 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	metadata := preview.Data[0].Metadata
	if metadata["parent_identity"] != "workspace://parent" || metadata["attachment_id"] != "file" || metadata["calendar_invitation"] != true {
		t.Fatalf("lost relationship metadata: %+v", metadata)
	}
	links, ok := metadata["related_resources"].([]any)
	if !ok || len(links) != 1 || links[0].(map[string]any)["id"] != "file" {
		t.Fatalf("lost Drive relationship: %+v", metadata)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"parent_identity":"workspace://parent"`) || !strings.Contains(string(encoded), `"related_resources"`) {
		t.Fatalf("JSON output lost metadata: %s", encoded)
	}
}

func TestSourceHostedScheduleActions(t *testing.T) {
	const (
		org      = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
		source   = "11111111-1111-4111-8111-111111111111"
		base     = "/api/v1/organizations/" + org + "/cloud/instances/" + instance + "/import-jobs/" + source
	)
	for _, action := range []struct {
		name string
		call func(*Client, context.Context, string, string, string) (map[string]any, error)
	}{
		{"pause", (*Client).PauseSource},
		{"resume", (*Client).ResumeSource},
	} {
		t.Run(action.name, func(t *testing.T) {
			t.Run("post_and_decode", func(t *testing.T) {
				var requests atomic.Int64
				want := map[string]any{
					"id": source, "name": "engineering-docs",
					"paused":   action.name == "pause",
					"schedule": map[string]any{"interval_seconds": float64(300)},
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodPost || r.URL.RequestURI() != base+"/"+action.name {
						t.Errorf("request = %s %s, want POST %s/%s", r.Method, r.URL.RequestURI(), base, action.name)
					}
					if got := r.Header.Get("Authorization"); got != "Bearer management-token" {
						t.Errorf("Authorization = %q, want Bearer management-token", got)
					}
					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(want); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
				if err != nil {
					t.Fatal(err)
				}
				got, err := action.call(client, context.Background(), org, instance, source)
				if err != nil {
					t.Fatalf("%s: %v", action.name, err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("job = %#v, want %#v", got, want)
				}
				if got := requests.Load(); got != 1 {
					t.Errorf("requests = %d, want 1", got)
				}
			})

			t.Run("invalid_ids_without_request", func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}))
				defer server.Close()
				client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
				if err != nil {
					t.Fatal(err)
				}
				for _, ids := range []struct {
					org, instance, source string
				}{
					{org, instance, "engineering-docs"},
					{org, instance, "../other?token=secret"},
					{org, instance, ""},
					{"not-an-org-uuid", instance, source},
					{org, "not-an-instance-uuid", source},
				} {
					if _, err := action.call(client, context.Background(), ids.org, ids.instance, ids.source); err == nil {
						t.Errorf("accepted invalid IDs: %+v", ids)
					}
				}
				if got := requests.Load(); got != 0 {
					t.Errorf("invalid IDs made %d requests, want 0", got)
				}
			})

			t.Run("not_found_api_error", func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					if err := json.NewEncoder(w).Encode(map[string]any{"error": "source not found"}); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
				if err != nil {
					t.Fatal(err)
				}
				_, err = action.call(client, context.Background(), org, instance, source)
				var apiErr *APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("error = %v (%T), want *APIError", err, err)
				}
				if apiErr.StatusCode != http.StatusNotFound {
					t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
				}
			})
		})
	}
}

func TestSourceConnectionValidateAndRenameContract(t *testing.T) {
	const (
		org      = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
		instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
		auth     = "631cd87b-035c-4e05-ae2e-421eae4c0f96"
		base     = "/api/v1/organizations/" + org + "/cloud/instances/" + instance + "/provider-auths/" + auth
	)
	for _, tc := range []struct {
		name, method, path, input, displayName string
	}{
		{"validate", http.MethodPost, base + "/validate", "", "company-google"},
		{"rename_trimmed", http.MethodPatch, base, "  eng-google  ", "eng-google"},
		{"rename_200_characters", http.MethodPatch, base, strings.Repeat("界", 200), strings.Repeat("界", 200)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			want := SourceConnection{ID: auth, Provider: "google_workspace", DisplayName: tc.displayName, Status: "needs_reauthorization", Enabled: true}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != tc.method || r.URL.RequestURI() != tc.path {
					t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.RequestURI(), tc.method, tc.path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer management-token" {
					t.Errorf("Authorization = %q", got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if tc.method == http.MethodPost {
					if len(body) != 0 {
						t.Errorf("validate body = %q, want no body", body)
					}
				} else {
					var got map[string]any
					if err := json.Unmarshal(body, &got); err != nil {
						t.Errorf("rename body: %v", err)
					} else if !reflect.DeepEqual(got, map[string]any{"display_name": tc.displayName}) {
						t.Errorf("rename body = %#v", got)
					}
					if got := r.Header.Get("Content-Type"); got != "application/json" {
						t.Errorf("Content-Type = %q", got)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(want); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			var got SourceConnection
			if tc.method == http.MethodPost {
				got, err = client.ValidateSourceConnection(context.Background(), org, instance, auth)
			} else {
				got, err = client.RenameSourceConnection(context.Background(), org, instance, auth, tc.input)
			}
			if err != nil {
				t.Errorf("connection action: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("summary = %+v, want %+v", got, want)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("requests = %d, want 1", got)
			}
		})
	}
}

func TestSourceConnectionActionsRejectNonUUIDWithoutRequest(t *testing.T) {
	const org = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
	const instance = "3bf7206e-c22c-47df-8126-366d4f53752d"
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "company-google", "../other?token=secret", "not-a-uuid"} {
		t.Run(id, func(t *testing.T) {
			if _, err := client.ValidateSourceConnection(context.Background(), org, instance, id); err == nil {
				t.Error("validate accepted non-UUID connection ID")
			}
			if _, err := client.RenameSourceConnection(context.Background(), org, instance, id, "eng-google"); err == nil {
				t.Error("rename accepted non-UUID connection ID")
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("invalid IDs made %d requests, want 0", got)
	}
}

func TestRenameSourceConnectionRejectsBadNamesWithoutRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "management-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, value string }{
		{"empty", ""},
		{"whitespace", " \t\n "},
		{"201_ascii", strings.Repeat("a", 201)},
		{"201_unicode", strings.Repeat("界", 201)},
		{"nul", "eng\x00google"},
		{"newline", "eng\ngoogle"},
		{"tab", "eng\tgoogle"},
		{"delete", "eng\x7fgoogle"},
		{"unicode_control", "eng\u0085google"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.RenameSourceConnection(context.Background(),
				"9a17e518-6274-4f79-8dff-80eb53e6d86c", "3bf7206e-c22c-47df-8126-366d4f53752d",
				"631cd87b-035c-4e05-ae2e-421eae4c0f96", tc.value)
			if err == nil {
				t.Errorf("accepted bad display name %q", tc.value)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("bad names made %d requests, want 0", got)
	}
}
