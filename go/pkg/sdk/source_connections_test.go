package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
