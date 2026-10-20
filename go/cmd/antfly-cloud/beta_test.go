package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBetaCommandsReuseSessionContextAndRefresh(t *testing.T) {
	const orgID, instanceID = "9a17e518-6274-4f79-8dff-80eb53e6d86c", "3bf7206e-c22c-47df-8126-366d4f53752d"
	for _, tc := range []struct {
		name                     string
		args                     []string
		endpoint, response, want string
	}{
		{"connections", []string{"beta", "connections", "list"}, "connections", `{"data":[{"id":"connection-id","provider":"github","display_name":"Engineering","auth_type":"github_app","enabled":true,"status":"active","config":{"token":"never-print-this"}}]}`, "Engineering"},
		{"connections JSON", []string{"beta", "connections", "list", "--json"}, "connections", `{"data":[{"id":"connection-id","provider":"github","display_name":"Engineering","auth_type":"github_app","enabled":true,"status":"active","secret":"never-print-this"}]}`, `"provider": "github"`},
		{"empty JSON", []string{"beta", "connections", "list", "--json"}, "connections", `{"data":[]}`, "[]"},
		{"capabilities", []string{"beta", "capabilities"}, "capabilities", `{"data":{"stage":"beta","operations":["connections.list"],"connection_providers":["github","s3"],"source_management":false,"search":false}}`, "Search: false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ANTFLY_CLOUD_TOKEN", "")
			refreshes, betaReads := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/token" {
					if r.Method != http.MethodPost {
						t.Errorf("token method = %s", r.Method)
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("refresh_token") != "old-refresh" {
						t.Errorf("unexpected refresh form")
					}
					refreshes++
					_, _ = w.Write([]byte(`{"id_token":"fresh-token","refresh_token":"fresh-refresh","expires_in":3600}`))
					return
				}
				if r.Method != http.MethodGet {
					t.Errorf("unexpected mutation: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer fresh-token" {
					t.Error("missing refreshed auth")
				}
				switch r.URL.Path {
				case "/api/v1/organizations":
					_, _ = w.Write([]byte(`{"data":[{"id":"` + orgID + `","name":"Acme","slug":"acme","status":"active"}]}`))
				case "/api/v1/organizations/" + orgID + "/cloud/instances":
					_, _ = w.Write([]byte(`{"data":[{"id":"` + instanceID + `","organization_id":"` + orgID + `","name":"Dev","slug":"dev","status":"ready"}]}`))
				case "/api/v1/organizations/" + orgID + "/cloud/instances/" + instanceID + "/beta/connected-sources/" + tc.endpoint:
					betaReads++
					_, _ = w.Write([]byte(tc.response))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			configPath := filepath.Join(t.TempDir(), "cloud.yaml")
			cfg := Config{APIURL: srv.URL + "/api/v1", Org: "acme", Instance: "dev", Auth: AuthConfig{Type: authTypeOIDC, Issuer: srv.URL, ClientID: defaultOIDCClientID, IDToken: "old-token", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour)}}
			if err := saveConfig(configPath, cfg); err != nil {
				t.Fatal(err)
			}
			var out, stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(append([]string{"--config", configPath}, tc.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if refreshes != 1 || betaReads != 1 {
				t.Fatalf("refreshes=%d betaReads=%d", refreshes, betaReads)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("unexpected output: %s", out.String())
			}
			for _, secret := range []string{"fresh-token", "fresh-refresh", "old-refresh", "never-print-this", "config"} {
				if strings.Contains(out.String()+stderr.String(), secret) {
					t.Errorf("output contained %s", secret)
				}
			}
			if strings.Contains(tc.name, "JSON") && !json.Valid(out.Bytes()) {
				t.Fatalf("invalid JSON output: %s", out.String())
			}
			updated, err := loadConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if updated.Auth.IDToken != "fresh-token" || updated.Auth.RefreshToken != "fresh-refresh" {
				t.Fatal("refreshed login was not persisted")
			}
		})
	}
}

func TestBetaUnavailableDoesNotBecomeEmptySuccess(t *testing.T) {
	err := connectedSourcesBetaError(&APIError{StatusCode: http.StatusNotFound})
	if !strings.Contains(err.Error(), "unavailable") || !strings.Contains(err.Error(), "server version") {
		t.Fatal(err)
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		original := &APIError{StatusCode: status}
		if got := connectedSourcesBetaError(original); got != original {
			t.Fatalf("status %d was obscured", status)
		}
	}
}
