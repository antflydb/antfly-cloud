package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialErrorsDoNotExposeSecrets(t *testing.T) {
	const secret = "credential-do-not-disclose"
	for _, response := range []struct {
		name   string
		status int
		body   string
	}{
		{"server echo", http.StatusBadRequest, `{"detail":"` + secret + `","token":"bearer-do-not-disclose"}`},
		{"malformed creation response", http.StatusCreated, `{"id":"` + secret + `"`},
	} {
		t.Run(response.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(response.status)
				_, _ = w.Write([]byte(response.body))
			}))
			defer srv.Close()
			client, err := NewClient(srv.URL, "bearer-do-not-disclose", srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.CreateProviderAuth(context.Background(), importTestOrg, importTestInstance, CreateProviderAuthRequest{Provider: "s3", Secret: secret})
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "bearer-do-not-disclose") {
				t.Fatalf("unsafe error: %v", err)
			}
			if response.status == http.StatusBadRequest {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != response.status {
					t.Fatalf("lost HTTP status: %v", err)
				}
			}
			_, err = client.CreateCloudAPIKey(context.Background(), importTestOrg, importTestInstance, CreateCloudAPIKeyRequest{Name: "reader"})
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "bearer-do-not-disclose") {
				t.Fatalf("unsafe key error: %v", err)
			}
		})
	}
}

func TestNamedTableOperationsRejectPathTraversal(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusNoContent) }))
	defer srv.Close()
	client, err := NewClient(srv.URL, "token", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"", " ", ".", "..", "../private", "a/b", `a\b`} {
		if _, err := client.GetTable(context.Background(), table); err == nil {
			t.Errorf("get accepted %q", table)
		}
		if _, err := client.QueryTable(context.Background(), table, []byte(`{}`)); err == nil {
			t.Errorf("query accepted %q", table)
		}
		if err := client.DropTable(context.Background(), table); err == nil {
			t.Errorf("drop accepted %q", table)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid table operations reached server: %d", calls)
	}
}
