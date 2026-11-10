package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antflydb/antfly/go/pkg/sources/management"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
)

type retireTestFixture struct {
	*runsTestFixture
	mu        sync.Mutex
	mutations []string
	polls     atomic.Int64
	key       string
}

func newRetireTestFixture(t *testing.T, purge bool) *retireTestFixture {
	t.Helper()
	f := &retireTestFixture{runsTestFixture: &runsTestFixture{}}
	f.factory = managementTestFactory(t, &f.output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, managementTestBase+"/") && (r.Method == http.MethodDelete || r.Method == http.MethodPost) {
			f.mu.Lock()
			f.mutations = append(f.mutations, r.Method+" "+r.URL.Path)
			f.mu.Unlock()
		}
		sourcePath := managementTestBase + "/sources/" + managementTestSource
		switch {
		case r.Method == http.MethodGet && r.URL.Path == managementTestBase+"/capabilities":
			managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "purge_sources": true, "per_source_runs": true})
		case r.Method == http.MethodGet && r.URL.Path == managementTestBase+"/sources/engineering-docs":
			w.WriteHeader(http.StatusNotFound)
			managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
		case r.Method == http.MethodGet && r.URL.Path == managementTestBase+"/sources":
			managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs"}}})
		case r.Method == http.MethodGet && r.URL.Path == sourcePath:
			managementTestJSON(t, w, map[string]any{"id": managementTestSource, "name": "engineering-docs", "state": "active"})
		case r.Method == http.MethodDelete && r.URL.Path == sourcePath:
			wantQuery := "purge=false"
			state := "retained"
			if purge {
				wantQuery, state = "purge=true", "purge_pending"
			}
			if got := r.URL.RawQuery; got != wantQuery {
				t.Errorf("DELETE query = %q, want %q", got, wantQuery)
			}
			managementTestJSON(t, w, management.Mutation{SourceId: managementTestSource, State: state})
		case r.Method == http.MethodPost && r.URL.Path == runsTestPath:
			if !purge {
				t.Error("archive must not submit a run")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding purge run input: %v", err)
			} else if body["operation"] != "purge" {
				t.Errorf("run input = %#v, want operation purge", body)
			}
			f.mu.Lock()
			f.key = r.Header.Get("Idempotency-Key")
			f.mu.Unlock()
			managementTestJSON(t, w, runsTestRunValue("pending"))
		case r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun:
			run := runsTestRunValue("running")
			if f.polls.Add(1) > 1 {
				run.State, run.Outcome, run.Authoritative = "completed", "success", true
			}
			managementTestJSON(t, w, run)
		default:
			t.Errorf("unexpected retirement request: %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	})
	return f
}

func TestSourcesArchiveRetainsIndexedContent(t *testing.T) {
	f := newRetireTestFixture(t, false)
	if err := f.execute(context.Background(), "archive", "engineering-docs"); err != nil {
		t.Fatalf("sources archive: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	want := []string{http.MethodDelete + " " + managementTestBase + "/sources/" + managementTestSource}
	if !reflect.DeepEqual(f.mutations, want) {
		t.Errorf("mutations = %v, want one archive DELETE and no POST runs", f.mutations)
	}
	if !strings.Contains(f.output.String(), "retained") {
		t.Errorf("archive output must explain retained content: %q", f.output.String())
	}
}

func TestSourcesPurgeRequiresYesWithoutMutation(t *testing.T) {
	f := newRetireTestFixture(t, true)
	err := f.execute(context.Background(), "purge", "engineering-docs")
	runsTestExit(t, err, 2)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.mutations) != 0 {
		t.Errorf("unconfirmed purge mutated source management: %v", f.mutations)
	}
}

func TestSourcesPurgeSubmissionAndWait(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wait, json bool
	}{
		{name: "submit"},
		{name: "wait", wait: true},
		{name: "json_wait", wait: true, json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetireTestFixture(t, true)
			args := []string{"purge", "engineering-docs", "--yes", "--idempotency-key", "purge_retry-7"}
			if tc.wait {
				args = append(args, "--wait", "--wait-timeout", "5s")
			}
			if tc.json {
				args = append(args, "--json")
			}
			if err := f.execute(context.Background(), args...); err != nil {
				t.Fatalf("sources purge: %v", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			want := []string{
				http.MethodDelete + " " + managementTestBase + "/sources/" + managementTestSource,
				http.MethodPost + " " + runsTestPath,
			}
			if !reflect.DeepEqual(f.mutations, want) {
				t.Errorf("mutations = %v, want DELETE then POST runs exactly once", f.mutations)
			}
			if f.key != "purge_retry-7" {
				t.Errorf("submitted idempotency key = %q", f.key)
			}
			wantPolls := int64(0)
			if tc.wait {
				wantPolls = 2
			}
			if got := f.polls.Load(); got != wantPolls {
				t.Errorf("run polls = %d, want %d", got, wantPolls)
			}
			if tc.json {
				var got commands.PurgeReceipt
				runsTestSingleJSON(t, &f.output, &got)
				want := commands.PurgeReceipt{
					Mutation:       management.Mutation{SourceId: managementTestSource, State: "purge_pending"},
					Run:            management.Run{Id: runsTestRun, SourceId: managementTestSource, State: "completed", Outcome: "success", Authoritative: true},
					IdempotencyKey: "purge_retry-7",
				}
				if got != want {
					t.Errorf("final purge receipt = %+v, want %+v", got, want)
				}
			} else {
				if !strings.Contains(f.output.String(), runsTestRun) {
					t.Errorf("purge receipt missing run ID: %q", f.output.String())
				}
				if tc.wait && !strings.Contains(f.output.String(), "Acquisition completed") {
					t.Errorf("purge wait did not report completion: %q", f.output.String())
				}
			}
		})
	}
}
