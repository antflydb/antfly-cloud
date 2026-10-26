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
