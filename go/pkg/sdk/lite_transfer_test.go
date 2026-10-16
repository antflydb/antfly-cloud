package sdk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type liteTransport func(*http.Request) (*http.Response, error)

func (f liteTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func liteResponse(status int, headers http.Header, body []byte) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(bytes.NewReader(body))}
}
func liteTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL+"/api/v1", "CONTROL-PLANE-TOKEN", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestLiteUploadResumesAmbiguousChunkWithoutSendingManagementCredentials(t *testing.T) {
	org, id := uuid.NewString(), uuid.NewString()
	data := bytes.Repeat([]byte("a"), int(liteTransferChunk)+19)
	digest := sha256.Sum256(data)
	source := filepath.Join(t.TempDir(), "snapshot.aflite")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	sealed := false
	client := liteTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer CONTROL-PLANE-TOKEN" {
			t.Error("missing management authentication")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/lite/imports"):
			var request StartLiteImportRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Sha256 != hex.EncodeToString(digest[:]) || request.SizeBytes != int64(len(data)) || request.IdempotencyKey != "stable-key" {
				t.Error("source identity changed")
			}
			fmt.Fprintf(w, `{"id":%q,"phase":"uploading"}`, id)
		case strings.HasSuffix(r.URL.Path, "/upload-session"):
			fmt.Fprint(w, `{"upload_url":"https://storage.googleapis.com/upload/storage/v1/b/private/o?upload_id=SECRET"}`)
		case strings.HasSuffix(r.URL.Path, "/seal"):
			sealed = true
			fmt.Fprintf(w, `{"id":%q,"phase":"queued"}`, id)
		default:
			t.Errorf("unexpected management route %s", r.URL.Path)
		}
	})
	var uploaded []byte
	interrupted := false
	transport := liteTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Error("management token leaked to object storage")
		}
		if strings.HasPrefix(r.Header.Get("Content-Range"), "bytes */") {
			if len(uploaded) == len(data) {
				return liteResponse(200, nil, nil), nil
			}
			headers := make(http.Header)
			if len(uploaded) > 0 {
				headers.Set("Range", fmt.Sprintf("bytes=0-%d", len(uploaded)-1))
			}
			return liteResponse(308, headers, nil), nil
		}
		var start, end, total int64
		fmt.Sscanf(r.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total)
		if start != int64(len(uploaded)) {
			t.Fatalf("wrong resume offset %d, committed %d", start, len(uploaded))
		}
		chunk, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !interrupted {
			interrupted = true
			uploaded = append(uploaded, chunk[:len(chunk)/2]...)
			return nil, fmt.Errorf("ambiguous failure involving %s", r.URL)
		}
		uploaded = append(uploaded, chunk...)
		return liteResponse(308, nil, nil), nil
	})
	op, err := client.UploadLiteSnapshot(t.Context(), org, source, "stable-key", TransferOptions{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	if string(op.Phase) != "queued" || !sealed || !bytes.Equal(uploaded, data) {
		t.Fatal("upload did not preserve and seal the complete source")
	}
}

func TestLiteDownloadVerifiesBeforeCreateAndProtectsExistingDestination(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			data := []byte("portable database snapshot")
			digest := sha256.Sum256(data)
			org, id := uuid.NewString(), uuid.NewString()
			client := liteTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"download_url":"https://storage.googleapis.com/private/export.aflite?generation=4&X-Goog-Signature=SECRET","sha256":%q,"size_bytes":%d}`, hex.EncodeToString(digest[:]), len(data))
			})
			transport := liteTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "" {
					t.Fatal("management credentials leaked")
				}
				body := bytes.Clone(data)
				if corrupt {
					body[0] = 'X'
				}
				headers := make(http.Header)
				headers.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(body)-1, len(body)))
				return liteResponse(206, headers, body), nil
			})
			path := filepath.Join(t.TempDir(), "returned.aflite")
			err := client.DownloadLiteFile(t.Context(), org, id, path, TransferOptions{HTTPClient: &http.Client{Transport: transport}})
			if corrupt {
				if err == nil || !strings.Contains(err.Error(), "checksum") {
					t.Fatalf("expected checksum failure, got %v", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("corrupt file was published")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatal("returned file differs")
			}
			if err := client.DownloadLiteFile(t.Context(), org, id, path, TransferOptions{}); err == nil || !strings.Contains(err.Error(), "destination exists") {
				t.Fatalf("existing destination not protected: %v", err)
			}
		})
	}
}
func TestTransferTransportErrorsDoNotExposeCapabilities(t *testing.T) {
	client := &http.Client{Transport: liteTransport(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("failed %s", r.URL) })}
	request, _ := http.NewRequestWithContext(context.Background(), "GET", "https://storage.googleapis.com/b/k?SECRET", nil)
	_, err := transferSend(client, request)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("capability leaked: %v", err)
	}
}
