package main

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
)

const processingTestJobBase = "/api/v1/organizations/" + managementTestOrg + "/cloud/instances/" + managementTestInstance + "/import-jobs/" + managementTestSource

type processingTestFixture struct {
	output       bytes.Buffer
	factory      stateFactory
	capabilities atomic.Int64
	sourceGets   atomic.Int64
	resolved     atomic.Int64
	runs         atomic.Int64
}

func newProcessingTestFixture(t *testing.T, jsonOut, artifactDiscovery bool, handler http.HandlerFunc) *processingTestFixture {
	t.Helper()
	f := &processingTestFixture{}
	f.factory = managementTestFactory(t, &f.output, jsonOut, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/runs") {
			f.runs.Add(1)
			t.Errorf("processing/schedule command touched acquisition runs: %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			if strings.HasPrefix(r.URL.Path, managementTestBase+"/sources") {
				f.sourceGets.Add(1)
			}
			switch r.URL.Path {
			case managementTestBase + "/capabilities":
				f.capabilities.Add(1)
				managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "artifact_discovery": artifactDiscovery})
				return
			case managementTestBase + "/sources/engineering-docs":
				w.WriteHeader(http.StatusNotFound)
				managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
				return
			case managementTestBase + "/sources":
				managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs"}}})
				return
			case managementTestBase + "/sources/" + managementTestSource:
				f.resolved.Add(1)
				managementTestJSON(t, w, map[string]any{
					"id": managementTestSource, "name": "engineering-docs", "state": "active", "search_version": 2,
					"streams": []any{map[string]any{"name": "documents"}},
					"artifacts": []any{map[string]any{
						"name": "original", "artifact": "original_bytes", "content_type": "application/pdf", "purpose": "download", "generated": false,
					}},
				})
				return
			}
		}
		handler(w, r)
	})
	return f
}

func processingTestUnexpected(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	t.Errorf("unexpected processing/schedule request: %s %s", r.Method, r.URL.RequestURI())
	http.NotFound(w, r)
}

func TestSourcesProcessingStatusByName(t *testing.T) {
	f := newProcessingTestFixture(t, false, true, func(w http.ResponseWriter, r *http.Request) {
		processingTestUnexpected(t, w, r)
	})
	if err := managementTestExecute(f.factory, &f.output, "processing", "status", "engineering-docs"); err != nil {
		t.Fatalf("sources processing status: %v", err)
	}
	if got := f.resolved.Load(); got != 1 {
		t.Errorf("resolved source GETs = %d, want 1", got)
	}
	for _, text := range []string{"engineering-docs", managementTestSource, "KIND", "unreported", "Acquisition runs are tracked separately"} {
		if !strings.Contains(f.output.String(), text) {
			t.Errorf("processing status missing %q: %s", text, f.output.String())
		}
	}
}

func TestSourcesArtifactsListJSON(t *testing.T) {
	f := newProcessingTestFixture(t, true, true, func(w http.ResponseWriter, r *http.Request) {
		processingTestUnexpected(t, w, r)
	})
	if err := managementTestExecute(f.factory, &f.output, "artifacts", "list", "engineering-docs"); err != nil {
		t.Fatalf("sources artifacts list: %v", err)
	}
	var got struct {
		SourceID string                  `json:"source_id"`
		Data     []commands.ArtifactView `json:"data"`
	}
	runsTestSingleJSON(t, &f.output, &got)
	want := []commands.ArtifactView{{
		Name: "original", Artifact: "original_bytes", ContentType: "application/pdf", Purpose: "download", State: "unreported",
	}}
	if got.SourceID != managementTestSource || !reflect.DeepEqual(got.Data, want) {
		t.Errorf("artifacts = %+v, want source_id %s and data %+v", got, managementTestSource, want)
	}
	if got := f.resolved.Load(); got != 1 {
		t.Errorf("resolved source GETs = %d, want 1", got)
	}
}

func TestSourcesArtifactsUnsupportedBeforeResolution(t *testing.T) {
	f := newProcessingTestFixture(t, true, false, func(w http.ResponseWriter, r *http.Request) {
		processingTestUnexpected(t, w, r)
	})
	err := managementTestExecute(f.factory, &f.output, "artifacts", "list", "engineering-docs")
	runsTestExit(t, err, 6)
	if got := f.capabilities.Load(); got != 1 {
		t.Errorf("capabilities GETs = %d, want 1", got)
	}
	if got := f.sourceGets.Load(); got != 0 {
		t.Errorf("unsupported artifact discovery made %d source GETs, want 0", got)
	}
}

func TestSourcesHostedScheduleActions(t *testing.T) {
	for _, action := range []struct {
		name, receipt string
	}{
		{"pause", "Paused"},
		{"resume", "Resumed"},
	} {
		for _, jsonOut := range []bool{false, true} {
			mode := "human"
			if jsonOut {
				mode = "json"
			}
			t.Run(action.name+"/"+mode, func(t *testing.T) {
				var posts atomic.Int64
				want := map[string]any{
					"id": managementTestSource, "name": "engineering-docs", "mode": "source",
					"paused": action.name == "pause", "schedule": map[string]any{"interval_seconds": float64(300)},
				}
				f := newProcessingTestFixture(t, jsonOut, true, func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.RequestURI() != processingTestJobBase+"/"+action.name {
						processingTestUnexpected(t, w, r)
						return
					}
					posts.Add(1)
					if got := r.Header.Get("Authorization"); got != "Bearer mgmt-token" {
						t.Errorf("Authorization = %q, want Bearer mgmt-token", got)
					}
					managementTestJSON(t, w, want)
				})
				if err := managementTestExecute(f.factory, &f.output, action.name, "engineering-docs"); err != nil {
					t.Fatalf("sources %s: %v", action.name, err)
				}
				if got := posts.Load(); got != 1 {
					t.Errorf("schedule POSTs = %d, want 1", got)
				}
				if got := f.resolved.Load(); got != 1 {
					t.Errorf("resolved source GETs = %d, want 1", got)
				}
				if got := f.runs.Load(); got != 0 {
					t.Errorf("acquisition run requests = %d, want 0", got)
				}
				if jsonOut {
					var got map[string]any
					runsTestSingleJSON(t, &f.output, &got)
					if !reflect.DeepEqual(got, want) {
						t.Errorf("job = %#v, want unchanged server object %#v", got, want)
					}
				} else {
					wantOutput := action.receipt + " source engineering-docs (" + managementTestSource + ").\n"
					if got := f.output.String(); got != wantOutput {
						t.Errorf("output = %q, want %q", got, wantOutput)
					}
				}
			})
		}
	}
}

func TestSourcesPauseForbidden(t *testing.T) {
	var posts atomic.Int64
	f := newProcessingTestFixture(t, false, true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.RequestURI() != processingTestJobBase+"/pause" {
			processingTestUnexpected(t, w, r)
			return
		}
		posts.Add(1)
		w.WriteHeader(http.StatusForbidden)
		managementTestJSON(t, w, map[string]any{"error": "forbidden"})
	})
	err := managementTestExecute(f.factory, &f.output, "pause", "engineering-docs")
	runsTestExit(t, err, 3)
	if got := posts.Load(); got != 1 {
		t.Errorf("pause POSTs = %d, want 1", got)
	}
	if strings.Contains(f.output.String(), "Paused source") {
		t.Errorf("failed pause printed success: %q", f.output.String())
	}
}

func TestSourcesProcessingAndScheduleHelp(t *testing.T) {
	var output bytes.Buffer
	factory := func(bool) (*appState, error) {
		t.Error("help must not resolve authenticated state")
		return nil, errors.New("help must not resolve authenticated state")
	}
	if err := managementTestExecute(factory, &output, "--help"); err != nil {
		t.Fatalf("sources --help: %v", err)
	}
	listed := make(map[string]bool)
	for _, line := range strings.Split(output.String(), "\n") {
		if fields := strings.Fields(line); len(fields) > 1 {
			listed[fields[0]] = true
		}
	}
	for _, name := range []string{"processing", "artifacts", "pause", "resume"} {
		if !listed[name] {
			t.Errorf("sources help missing %s: %s", name, output.String())
		}
	}
	for _, name := range []string{"pause", "resume"} {
		t.Run(name+"_schedule_help", func(t *testing.T) {
			command, _, err := newSourcesCommand(factory).Find([]string{name})
			if err != nil || command == nil || command.Name() != name {
				t.Fatalf("sources %s command unavailable: %v", name, err)
			}
			if !strings.Contains(strings.ToLower(command.Short), "hosted schedule") {
				t.Errorf("short help must describe the hosted schedule: %q", command.Short)
			}
		})
	}
}
