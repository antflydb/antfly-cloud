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
	const bearer = "bearer-do-not-disclose"
	const credentialID = "39daa387-43ee-420c-946e-4d05d132b60d"
	replace := true
	operations := []struct {
		name string
		call func(*Client) (*ProviderAuth, error)
	}{
		{"create", func(client *Client) (*ProviderAuth, error) {
			return client.CreateProviderAuth(context.Background(), importTestOrg, importTestInstance, CreateProviderAuthRequest{Provider: "s3", Secret: secret})
		}},
		{"get", func(client *Client) (*ProviderAuth, error) {
			return client.GetProviderAuth(context.Background(), importTestOrg, importTestInstance, credentialID)
		}},
		{"update", func(client *Client) (*ProviderAuth, error) {
			value := secret
			return client.UpdateProviderAuth(context.Background(), importTestOrg, importTestInstance, credentialID, UpdateProviderAuthRequest{Secret: &value, ReplaceSecret: &replace})
		}},
		{"validate", func(client *Client) (*ProviderAuth, error) {
			return client.ValidateProviderAuth(context.Background(), importTestOrg, importTestInstance, credentialID)
		}},
	}
	for _, response := range []struct {
		name   string
		status int
		body   string
	}{
		{"server echo", http.StatusBadRequest, `{"detail":"` + secret + `","token":"` + bearer + `"}`},
		{"malformed successful response", http.StatusCreated, `{"id":"` + secret + `","token":"` + bearer + `"`},
		{"malformed successful timestamp", http.StatusOK, `{"id":"` + secret + `","created_at":"` + secret + ` ` + bearer + `"}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(response.status)
				_, _ = w.Write([]byte(response.body))
			}))
			defer srv.Close()
			client, err := NewClient(srv.URL, bearer, srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					result, err := operation.call(client)
					if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), bearer) {
						t.Fatalf("unsafe error: %v", err)
					}
					if result != nil {
						t.Fatalf("returned metadata from failed sensitive response: %#v", result)
					}
					if response.status == http.StatusBadRequest {
						var apiErr *APIError
						if !errors.As(err, &apiErr) || apiErr.StatusCode != response.status {
							t.Fatalf("lost HTTP status: %v", err)
						}
					}
				})
			}
			_, err = client.CreateCloudAPIKey(context.Background(), importTestOrg, importTestInstance, CreateCloudAPIKeyRequest{Name: "reader"})
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), bearer) {
				t.Fatalf("unsafe key error: %v", err)
			}
		})
	}
}

func TestProviderAuthLifecycleRejectsInvalidIDs(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL, "token", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	operations := []struct {
		name string
		call func(context.Context, string, string, string) (*ProviderAuth, error)
	}{
		{"get", client.GetProviderAuth},
		{"update", func(ctx context.Context, org, instance, id string) (*ProviderAuth, error) {
			enabled := false
			return client.UpdateProviderAuth(ctx, org, instance, id, UpdateProviderAuthRequest{Enabled: &enabled})
		}},
		{"validate", client.ValidateProviderAuth},
	}
	const credentialID = "39daa387-43ee-420c-946e-4d05d132b60d"
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, ids := range []struct {
				name     string
				org      string
				instance string
				id       string
			}{
				{"invalid organization", "not-a-uuid", importTestInstance, credentialID},
				{"missing organization", "", importTestInstance, credentialID},
				{"invalid instance", importTestOrg, "not-a-uuid", credentialID},
				{"missing instance", importTestOrg, "", credentialID},
				{"invalid credential", importTestOrg, importTestInstance, "not-a-uuid"},
				{"missing credential", importTestOrg, importTestInstance, ""},
				{"credential traversal", importTestOrg, importTestInstance, "../private"},
			} {
				t.Run(ids.name, func(t *testing.T) {
					result, err := operation.call(context.Background(), ids.org, ids.instance, ids.id)
					if err == nil || result != nil {
						t.Fatalf("invalid IDs returned result=%#v error=%v", result, err)
					}
				})
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid credential operations reached server: %d", calls)
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
