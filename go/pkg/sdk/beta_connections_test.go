package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBetaConnectionsRejectIncompleteResponsesAndRedactErrors(t *testing.T) {
	const orgID, instanceID = "9a17e518-6274-4f79-8dff-80eb53e6d86c", "3bf7206e-c22c-47df-8126-366d4f53752d"
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"missing envelope", 200, `{}`}, {"null list", 200, `{"data":null}`},
		{"truncated JSON", 200, `{"data":[`}, {"oversized", 200, strings.Repeat(" ", 1<<20) + `{"data":[]}`},
		{"error body", 403, `{"detail":"private-token"}`}, {"unexpected success status", 204, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/organizations/"+orgID+"/cloud/instances/"+instanceID+"/beta/connected-sources/connections" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authentication")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client, err := NewClient(srv.URL+"/api/v1/", "test-token", srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.BetaSourceConnections(context.Background(), orgID, instanceID)
			if err == nil {
				t.Fatal("incomplete response accepted")
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatal("server error body exposed")
			}
			if tc.status != http.StatusOK {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status {
					t.Fatalf("status not retained: %v", err)
				}
			}
		})
	}
}

func TestBetaRequestRejectsInvalidScopeBeforeTransport(t *testing.T) {
	client, err := NewClient("http://unused.invalid/api/v1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.BetaSourceConnections(context.Background(), "../escape", "invalid"); err == nil {
		t.Fatal("invalid scope accepted")
	}
}
