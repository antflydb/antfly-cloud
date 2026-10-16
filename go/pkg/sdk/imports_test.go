package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const importTestOrg = "9a17e518-6274-4f79-8dff-80eb53e6d86c"
const importTestInstance = "3bf7206e-c22c-47df-8126-366d4f53752d"
const importTestID = "459294c6-f229-4fa0-a6d2-7f5cbec020f3"
const importTestRun = "f36960bb-e39c-496d-83ff-b127c21b1bda"

func TestImportRunResultRejectsFalseSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, status, outcome string
		terminal, failure     bool
	}{
		{"pending", "pending", "", false, false},
		{"running", "running", "partial", false, false},
		{"success", "complete", "success", true, false},
		{"partial", "complete", "partial", true, true},
		{"failed outcome", "complete", "failed", true, true},
		{"historical missing outcome", "complete", "", true, true},
		{"unknown outcome", "complete", "unexpected", true, true},
		{"legacy failure", "failed", "", true, true},
		{"unknown status", "unexpected", "success", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal, err := ImportRunResult(ImportRun{ID: importTestRun, Status: tc.status, Diagnostics: RunDiagnostics{Outcome: tc.outcome}})
			if terminal != tc.terminal || (err != nil) != tc.failure {
				t.Fatalf("terminal=%v err=%v", terminal, err)
			}
		})
	}
}

func TestWaitImportRunPagesForSpecificRunAndPreservesDiagnostics(t *testing.T) {
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var runs []ImportRun
		if r.URL.Query().Get("offset") == "0" {
			// More recent successful runs must not terminate this caller's wait.
			for i := range 100 {
				runs = append(runs, ImportRun{ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", i), Status: "complete", Diagnostics: RunDiagnostics{Outcome: "success"}})
			}
		} else if r.URL.Query().Get("offset") == "100" {
			polls++
			run := ImportRun{ID: importTestRun, ImportID: importTestID, Status: "running"}
			if polls == 2 {
				run.Status = "complete"
				run.Diagnostics = RunDiagnostics{Outcome: "partial", Diagnostics: []RunDiagnostic{{Code: "document_extraction_failed", Detail: "cannot extract", Source: &RunDiagnosticSource{Bucket: "docs", Key: "bad.pdf"}}}}
			}
			runs = []ImportRun{run}
		} else {
			t.Errorf("unexpected offset %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": runs})
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL, "token", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	run, err := client.WaitImportRun(context.Background(), importTestOrg, importTestInstance, importTestID, importTestRun, time.Millisecond)
	if err == nil || run == nil || run.ID != importTestRun || run.Diagnostics.Outcome != "partial" {
		t.Fatalf("run=%#v err=%v", run, err)
	}
	if run.Diagnostics.Diagnostics[0].Source.Key != "bad.pdf" || run.Diagnostics.Diagnostics[0].Detail != "cannot extract" {
		t.Fatalf("lost diagnostics: %#v", run.Diagnostics)
	}
}

func TestWaitImportRunTimeoutAndCancellation(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "cancel"}[cancelWait], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if !cancelWait {
				var cancelTimeout context.CancelFunc
				ctx, cancelTimeout = context.WithTimeout(ctx, 500*time.Millisecond)
				defer cancelTimeout()
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []ImportRun{{ID: importTestRun, Status: "running"}}})
				if cancelWait {
					cancel()
				}
			}))
			defer srv.Close()
			client, err := NewClient(srv.URL, "token", srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			run, err := client.WaitImportRun(ctx, importTestOrg, importTestInstance, importTestID, importTestRun, time.Hour)
			want := context.DeadlineExceeded
			if cancelWait {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("err=%v want=%v", err, want)
			}
			// A cancel can race the initial response; a timeout must preserve it.
			if !cancelWait && (run == nil || run.Status != "running") {
				t.Fatalf("lost last run: %#v", run)
			}
		})
	}
}

func TestWaitImportRunDoesNotAcceptUnrelatedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []ImportRun{{ID: "different-run", Status: "complete", Diagnostics: RunDiagnostics{Outcome: "success"}}}})
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL, "token", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	run, err := client.WaitImportRun(context.Background(), importTestOrg, importTestInstance, importTestID, importTestRun, time.Millisecond)
	if err == nil || run != nil {
		t.Fatalf("run=%#v err=%v", run, err)
	}
}

func TestWaitImportRunRejectsForeignImportOwnership(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use the public wire field directly so a decoder regression cannot omit ownership.
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
			"id": importTestRun, "import_id": importTestInstance, "status": "complete",
			"diagnostics": map[string]any{"outcome": "success"},
		}}})
	}))
	defer srv.Close()
	client, err := NewClient(srv.URL, "token", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	run, err := client.WaitImportRun(context.Background(), importTestOrg, importTestInstance, importTestID, importTestRun, time.Millisecond)
	if err == nil || run != nil {
		t.Fatalf("accepted another import's successful run: run=%#v err=%v", run, err)
	}
}
