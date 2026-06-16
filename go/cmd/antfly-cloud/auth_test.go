package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRefreshConfigIfNeeded(t *testing.T) {
	var sawRefresh bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("client_id") != "cloudaf-cli" {
			t.Fatalf("unexpected form: %v", r.Form)
		}
		sawRefresh = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id_token":      "new-id",
			"refresh_token": "new-refresh",
			"expires_in":    3600,
			"token_type":    "Bearer",
		})
	}))
	defer srv.Close()

	cfg := Config{APIURL: "https://platform-dev.antfly.io/api/v1", Auth: AuthConfig{
		Type:         authTypeOIDC,
		Issuer:       srv.URL,
		ClientID:     defaultOIDCClientID,
		IDToken:      "old-id",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}}
	got, refreshed, err := refreshConfigIfNeeded(context.Background(), cfg, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed || !sawRefresh {
		t.Fatalf("expected refresh")
	}
	if got.Auth.IDToken != "new-id" || got.Auth.RefreshToken != "new-refresh" || got.bearerToken() != "new-id" {
		t.Fatalf("unexpected cfg: %#v", got.Auth)
	}
}

func TestRefreshConfigIfNeededSkipsFreshToken(t *testing.T) {
	cfg := Config{Auth: AuthConfig{Type: authTypeOIDC, IDToken: "id", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}}
	got, refreshed, err := refreshConfigIfNeeded(context.Background(), cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed || got.Auth.IDToken != "id" {
		t.Fatalf("unexpected refresh: refreshed=%v cfg=%#v", refreshed, got.Auth)
	}
}

func TestDefaultOrgForLogin(t *testing.T) {
	orgA := Organization{ID: "org-a", Slug: "alpha"}
	orgB := Organization{ID: "org-b", Slug: "beta"}
	tests := []struct {
		name    string
		current string
		orgs    []Organization
		want    string
	}{
		{"empty one org", "", []Organization{orgA}, "org-a"},
		{"empty multiple orgs", "", []Organization{orgA, orgB}, ""},
		{"existing id still visible", "org-a", []Organization{orgA, orgB}, "org-a"},
		{"existing slug still visible", "alpha", []Organization{orgA, orgB}, "alpha"},
		{"stale with one org", "stale-org", []Organization{orgA}, "org-a"},
		{"stale with multiple orgs", "stale-org", []Organization{orgA, orgB}, ""},
		{"stale with no orgs", "stale-org", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultOrgForLogin(tt.current, tt.orgs); got != tt.want {
				t.Fatalf("defaultOrgForLogin(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}
