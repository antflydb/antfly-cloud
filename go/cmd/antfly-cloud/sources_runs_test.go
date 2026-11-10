package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/antflydb/antfly/go/pkg/sources/management"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
)

const (
	runsTestRun    = "22222222-2222-4222-8222-222222222222"
	runsTestPath   = managementTestBase + "/sources/" + managementTestSource + "/runs"
	runsTestResume = "Resume watching with: antfly-cloud sources runs wait " + managementTestSource + " " + runsTestRun
)

type runsTestFixture struct {
	output          bytes.Buffer
	factory         stateFactory
	cancelSupported bool
	posts           atomic.Int64
	polls           atomic.Int64
	cancels         atomic.Int64
	legacy          atomic.Int64
}

func newRunsTestFixture(t *testing.T, handler http.HandlerFunc) *runsTestFixture {
	t.Helper()
	f := &runsTestFixture{cancelSupported: true}
	f.factory = managementTestFactory(t, &f.output, false, time.Second, nil, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/import-jobs") {
			f.legacy.Add(1)
			t.Errorf("run command used legacy endpoint: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			f.cancels.Add(1)
		}
		if r.Method == http.MethodPost && r.URL.Path == runsTestPath {
			f.posts.Add(1)
		}
		if r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun {
			f.polls.Add(1)
		}
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case managementTestBase + "/capabilities":
				managementTestJSON(t, w, map[string]any{"version": 1, "runtime": "colony", "per_source_runs": true, "cancel_runs": f.cancelSupported})
				return
			case managementTestBase + "/sources/engineering-docs":
				w.WriteHeader(http.StatusNotFound)
				managementTestJSON(t, w, map[string]any{"version": 1, "code": "source_not_found"})
				return
			case managementTestBase + "/sources":
				managementTestJSON(t, w, map[string]any{"data": []any{map[string]any{"id": managementTestSource, "name": "engineering-docs"}}})
				return
			case managementTestBase + "/sources/" + managementTestSource:
				managementTestJSON(t, w, map[string]any{"id": managementTestSource, "name": "engineering-docs", "state": "active"})
				return
			}
		}
		handler(w, r)
	})
	return f
}

// Keep the real root flags while injecting M1's isolated, authenticated state.
func (f *runsTestFixture) execute(ctx context.Context, args ...string) error {
	root := newRootCommand(&f.output, io.Discard)
	for _, child := range root.Commands() {
		if child.Name() == "sources" {
			root.RemoveCommand(child)
			break
		}
	}
	root.AddCommand(newSourcesCommand(func(requireToken bool) (*appState, error) {
		st, err := f.factory(requireToken)
		if err != nil {
			return nil, err
		}
		st.out.JSON, _ = root.PersistentFlags().GetBool("json")
		return st, nil
	}))
	root.SetArgs(append([]string{"sources"}, args...))
	return root.ExecuteContext(ctx)
}

func runsTestRunValue(state management.RunState) management.Run {
	return management.Run{Id: runsTestRun, SourceId: managementTestSource, State: state}
}

func runsTestUnexpected(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	t.Errorf("unexpected run request: %s %s", r.Method, r.URL.RequestURI())
	http.NotFound(w, r)
}

func runsTestSingleJSON(t *testing.T, output *bytes.Buffer, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decoding stdout: %v; output: %q", err, output.String())
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout must contain exactly one JSON document: second Decode = %v; output: %q", err, output.String())
	}
}

func runsTestExit(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("command succeeded, want exit %d", want)
	}
	if got := errorExitCode(err); got != want {
		t.Errorf("exit = %d, want %d; error: %v", got, want, err)
	}
}

func (f *runsTestFixture) assertNoCancel(t *testing.T) {
	t.Helper()
	if got := f.cancels.Load(); got != 0 {
		t.Errorf("watching issued %d cancellation requests", got)
	}
}

func TestSourcesRunRestart(t *testing.T) {
	var key string
	f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != runsTestPath {
			runsTestUnexpected(t, w, r)
			return
		}
		key = r.Header.Get("Idempotency-Key")
		if !regexp.MustCompile(`^cli-[0-9a-f]{32}$`).MatchString(key) {
			t.Errorf("generated idempotency key = %q", key)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding run input: %v", err)
		} else if body["restart"] != true {
			t.Errorf("run input = %#v, want restart:true", body)
		}
		managementTestJSON(t, w, runsTestRunValue("pending"))
	})
	if err := f.execute(context.Background(), "run", "engineering-docs", "--restart"); err != nil {
		t.Fatalf("sources run --restart: %v", err)
	}
	if got := f.posts.Load(); got != 1 {
		t.Errorf("run POSTs = %d, want 1", got)
	}
	if !strings.Contains(f.output.String(), runsTestRun) {
		t.Errorf("receipt missing run ID: %q", f.output.String())
	}
}

func TestSourcesRunIdempotencyKey(t *testing.T) {
	for _, key := range []string{"retry-1", "bad key"} {
		t.Run(key, func(t *testing.T) {
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != runsTestPath {
					runsTestUnexpected(t, w, r)
					return
				}
				if got := r.Header.Get("Idempotency-Key"); got != key {
					t.Errorf("idempotency key = %q, want verbatim %q", got, key)
				}
				managementTestJSON(t, w, runsTestRunValue("pending"))
			})
			err := f.execute(context.Background(), "run", "engineering-docs", "--idempotency-key", key)
			if key == "bad key" {
				runsTestExit(t, err, 2)
				if got := f.posts.Load(); got != 0 {
					t.Errorf("invalid key submitted %d runs", got)
				}
			} else {
				if err != nil {
					t.Fatalf("sources run with supplied key: %v", err)
				}
				if got := f.posts.Load(); got != 1 {
					t.Errorf("run POSTs = %d, want 1", got)
				}
			}
		})
	}
}

func TestSourcesRunWait(t *testing.T) {
	for _, machine := range []bool{false, true} {
		name := "human"
		if machine {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			var polls atomic.Int64
			var submittedKey string
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == runsTestPath:
					submittedKey = r.Header.Get("Idempotency-Key")
					managementTestJSON(t, w, runsTestRunValue("pending"))
				case r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun:
					run := runsTestRunValue("running")
					if polls.Add(1) > 1 {
						run.State, run.Outcome, run.Authoritative = "completed", "success", true
					}
					managementTestJSON(t, w, run)
				default:
					runsTestUnexpected(t, w, r)
				}
			})
			args := []string{"run", "engineering-docs", "--wait", "--wait-timeout", "5s"}
			if machine {
				args = append(args, "--json")
			}
			if err := f.execute(context.Background(), args...); err != nil {
				t.Fatalf("sources run --wait: %v", err)
			}
			if f.posts.Load() != 1 || polls.Load() != 2 {
				t.Errorf("POSTs = %d, polls = %d; want one submission and running/completed polls", f.posts.Load(), polls.Load())
			}
			f.assertNoCancel(t)
			if machine {
				var receipt commands.RunReceipt
				runsTestSingleJSON(t, &f.output, &receipt)
				if receipt.Run != (management.Run{Id: runsTestRun, SourceId: managementTestSource, State: "completed", Outcome: "success", Authoritative: true}) {
					t.Errorf("final receipt run = %+v", receipt.Run)
				}
				if receipt.IdempotencyKey == "" || receipt.IdempotencyKey != submittedKey {
					t.Errorf("receipt key = %q, submitted key = %q", receipt.IdempotencyKey, submittedKey)
				}
			} else {
				for _, text := range []string{runsTestRun, "Acquisition completed"} {
					if !strings.Contains(f.output.String(), text) {
						t.Errorf("output missing %q: %q", text, f.output.String())
					}
				}
			}
		})
	}
}

func TestSourcesRunWaitTimeout(t *testing.T) {
	for _, machine := range []bool{false, true} {
		name := "human"
		if machine {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == runsTestPath:
					managementTestJSON(t, w, runsTestRunValue("pending"))
				case r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun:
					managementTestJSON(t, w, runsTestRunValue("running"))
				default:
					runsTestUnexpected(t, w, r)
				}
			})
			args := []string{"run", "engineering-docs", "--wait", "--wait-timeout", "50ms"}
			if machine {
				args = append(args, "--json")
			}
			runsTestExit(t, f.execute(context.Background(), args...), 7)
			f.assertNoCancel(t)
			if f.polls.Load() == 0 {
				t.Error("wait timed out without polling the hosted run")
			}
			if machine {
				var receipt commands.RunReceipt
				runsTestSingleJSON(t, &f.output, &receipt)
				if receipt.Run != runsTestRunValue("running") || receipt.IdempotencyKey == "" {
					t.Errorf("timeout must preserve last observation and key: %+v", receipt)
				}
			} else if !strings.Contains(f.output.String(), runsTestResume) {
				t.Errorf("missing resume hint: %q", f.output.String())
			}
		})
	}
}

func TestSourcesRunWaitInterrupt(t *testing.T) {
	for _, machine := range []bool{false, true} {
		name := "human"
		if machine {
			name = "json_falls_back_to_receipt"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == runsTestPath:
					managementTestJSON(t, w, runsTestRunValue("pending"))
				case r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun:
					cancel()
					http.Error(w, "watch interrupted before any observation", http.StatusServiceUnavailable)
				default:
					runsTestUnexpected(t, w, r)
				}
			})
			args := []string{"run", "engineering-docs", "--wait", "--wait-timeout", "5s"}
			if machine {
				args = append(args, "--json")
			}
			runsTestExit(t, f.execute(ctx, args...), 130)
			if f.polls.Load() != 1 {
				t.Errorf("polls = %d, want interruption on first poll", f.polls.Load())
			}
			f.assertNoCancel(t)
			if machine {
				var receipt commands.RunReceipt
				runsTestSingleJSON(t, &f.output, &receipt)
				if receipt.Run != runsTestRunValue("pending") || receipt.IdempotencyKey == "" {
					t.Errorf("no observation must preserve start receipt: %+v", receipt)
				}
			} else if !strings.Contains(f.output.String(), runsTestResume) {
				t.Errorf("missing interrupt resume hint: %q", f.output.String())
			}
		})
	}
}

func TestSourcesRunWaitFailed(t *testing.T) {
	for _, machine := range []bool{false, true} {
		name := "human"
		if machine {
			name = "json"
		}
		t.Run(name, func(t *testing.T) {
			failed := runsTestRunValue("failed")
			failed.Code = "provider_denied"
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == runsTestPath:
					managementTestJSON(t, w, runsTestRunValue("pending"))
				case r.Method == http.MethodGet && r.URL.Path == runsTestPath+"/"+runsTestRun:
					managementTestJSON(t, w, failed)
				default:
					runsTestUnexpected(t, w, r)
				}
			})
			args := []string{"run", "engineering-docs", "--wait", "--wait-timeout", "5s"}
			if machine {
				args = append(args, "--json")
			}
			runsTestExit(t, f.execute(context.Background(), args...), 8)
			f.assertNoCancel(t)
			if machine {
				var receipt commands.RunReceipt
				runsTestSingleJSON(t, &f.output, &receipt)
				if receipt.Run != failed || receipt.IdempotencyKey == "" {
					t.Errorf("failure receipt = %+v, want failed run and submission key", receipt)
				}
			}
		})
	}
}

func TestSourcesRunsList(t *testing.T) {
	const olderRun = "33333333-3333-4333-8333-333333333333"
	for _, all := range []bool{true, false} {
		name := "all"
		if !all {
			name = "limit_and_cursor"
		}
		t.Run(name, func(t *testing.T) {
			var pages atomic.Int64
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != runsTestPath {
					runsTestUnexpected(t, w, r)
					return
				}
				page := pages.Add(1)
				wantLimit, wantCursor := "20", ""
				if !all {
					wantLimit, wantCursor = "7", "resume+cursor/="
				} else if page == 2 {
					wantCursor = "next+cursor/="
				}
				if r.URL.Query().Get("limit") != wantLimit || r.URL.Query().Get("cursor") != wantCursor {
					t.Errorf("pagination query = %q, want limit=%s cursor=%q", r.URL.RawQuery, wantLimit, wantCursor)
				}
				if page == 1 {
					managementTestJSON(t, w, management.RunPage{Data: []management.Run{runsTestRunValue("running")}, NextCursor: "next+cursor/="})
				} else if all && page == 2 {
					managementTestJSON(t, w, management.RunPage{Data: []management.Run{{Id: olderRun, SourceId: managementTestSource, State: "completed"}}})
				} else {
					runsTestUnexpected(t, w, r)
				}
			})
			args := []string{"runs", "list", "engineering-docs", "--json"}
			if all {
				args = append(args, "--all")
			} else {
				args = append(args, "--limit", "7", "--cursor", "resume+cursor/=")
			}
			if err := f.execute(context.Background(), args...); err != nil {
				t.Fatalf("sources runs list: %v", err)
			}
			var result management.RunPage
			runsTestSingleJSON(t, &f.output, &result)
			if all {
				if pages.Load() != 2 || len(result.Data) != 2 || result.Data[0].Id != runsTestRun || result.Data[1].Id != olderRun || result.NextCursor != "" {
					t.Errorf("all pages = %d, result = %+v", pages.Load(), result)
				}
			} else if pages.Load() != 1 || len(result.Data) != 1 || result.Data[0].Id != runsTestRun || result.NextCursor != "next+cursor/=" {
				t.Errorf("single page = %d, result = %+v", pages.Load(), result)
			}
		})
	}
}

func TestSourcesRunsInspect(t *testing.T) {
	f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != runsTestPath+"/"+runsTestRun {
			runsTestUnexpected(t, w, r)
			return
		}
		managementTestJSON(t, w, runsTestRunValue("running"))
	})
	if err := f.execute(context.Background(), "runs", "inspect", "engineering-docs", runsTestRun, "--json"); err != nil {
		t.Fatalf("sources runs inspect: %v", err)
	}
	var run management.Run
	runsTestSingleJSON(t, &f.output, &run)
	if f.polls.Load() != 1 || run != runsTestRunValue("running") {
		t.Errorf("inspection GETs = %d, run = %+v", f.polls.Load(), run)
	}
}

func TestSourcesRunsCancel(t *testing.T) {
	for _, supported := range []bool{true, false} {
		name := "supported"
		if !supported {
			name = "unsupported"
		}
		t.Run(name, func(t *testing.T) {
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != runsTestPath+"/"+runsTestRun+"/cancel" {
					runsTestUnexpected(t, w, r)
					return
				}
				managementTestJSON(t, w, runsTestRunValue("cancelled"))
			})
			f.cancelSupported = supported
			err := f.execute(context.Background(), "runs", "cancel", "engineering-docs", runsTestRun)
			if !supported {
				runsTestExit(t, err, 6)
				f.assertNoCancel(t)
				return
			}
			if err != nil {
				t.Fatalf("sources runs cancel: %v", err)
			}
			if f.cancels.Load() != 1 || !strings.Contains(f.output.String(), runsTestRun) {
				t.Errorf("cancel requests = %d, output = %q", f.cancels.Load(), f.output.String())
			}
		})
	}
}

func TestSourcesRunsWait(t *testing.T) {
	for _, mode := range []string{"human", "json", "timeout", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completed := runsTestRunValue("completed")
			completed.Outcome, completed.Authoritative = "success", true
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != runsTestPath+"/"+runsTestRun {
					runsTestUnexpected(t, w, r)
					return
				}
				switch mode {
				case "timeout":
					managementTestJSON(t, w, runsTestRunValue("running"))
				case "interrupt":
					cancel()
					http.Error(w, "watch interrupted", http.StatusServiceUnavailable)
				default:
					managementTestJSON(t, w, completed)
				}
			})
			args := []string{"runs", "wait", managementTestSource, runsTestRun, "--wait-timeout", "5s"}
			if mode == "json" {
				args = append(args, "--json")
			} else if mode == "timeout" {
				args = append(args, "--wait-timeout", "50ms")
			}
			err := f.execute(ctx, args...)
			f.assertNoCancel(t)
			if mode == "timeout" || mode == "interrupt" {
				wantExit := 7
				if mode == "interrupt" {
					wantExit = 130
				}
				runsTestExit(t, err, wantExit)
				if !strings.Contains(f.output.String(), runsTestResume) {
					t.Errorf("missing resume hint: %q", f.output.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("sources runs wait: %v", err)
			}
			if f.polls.Load() != 1 || f.posts.Load() != 0 {
				t.Errorf("wait GETs = %d, start POSTs = %d", f.polls.Load(), f.posts.Load())
			}
			if mode == "json" {
				var run management.Run
				runsTestSingleJSON(t, &f.output, &run)
				if run != completed {
					t.Errorf("bare final run = %+v, want %+v", run, completed)
				}
			} else if !strings.Contains(f.output.String(), "Acquisition completed") {
				t.Errorf("missing acquisition result: %q", f.output.String())
			}
		})
	}
}

func TestSourcesSyncUsesManagementRuns(t *testing.T) {
	for _, flags := range []bool{false, true} {
		name := "plain"
		if flags {
			name = "run_flags"
		}
		t.Run(name, func(t *testing.T) {
			f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != runsTestPath {
					runsTestUnexpected(t, w, r)
					return
				}
				if flags {
					var body management.RunInput
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decoding sync run input: %v", err)
					} else if !body.Restart {
						t.Error("sync --restart did not restart the management run")
					}
					if got := r.Header.Get("Idempotency-Key"); got != "sync-retry" {
						t.Errorf("sync key = %q, want sync-retry", got)
					}
				}
				managementTestJSON(t, w, runsTestRunValue("pending"))
			})
			args := []string{"sync", "engineering-docs"}
			if flags {
				args = append(args, "--restart", "--idempotency-key", "sync-retry")
			}
			err := f.execute(context.Background(), args...)
			if got := f.legacy.Load(); got != 0 {
				t.Errorf("sync issued %d legacy import-job requests", got)
			}
			if err != nil {
				t.Fatalf("sources sync: %v", err)
			}
			if got := f.posts.Load(); got != 1 {
				t.Errorf("sync management run POSTs = %d, want 1", got)
			}
		})
	}
}

func TestSourcesRunWaitTimeoutInvalid(t *testing.T) {
	for _, action := range []string{"run", "runs wait"} {
		for _, timeout := range []string{"0", "-1s"} {
			t.Run(action+"/"+timeout, func(t *testing.T) {
				f := newRunsTestFixture(t, func(w http.ResponseWriter, r *http.Request) {
					runsTestUnexpected(t, w, r)
				})
				args := []string{"run", "engineering-docs", "--wait"}
				if action == "runs wait" {
					args = []string{"runs", "wait", managementTestSource, runsTestRun}
				}
				args = append(args, "--wait-timeout", timeout)
				err := f.execute(context.Background(), args...)
				runsTestExit(t, err, 2)
				var failure *commands.Error
				if !errors.As(err, &failure) || failure.Code != commands.CodeInvalid {
					t.Errorf("invalid wait timeout error = %T %v, want commands.CodeInvalid", err, err)
				}
				if f.posts.Load() != 0 || f.polls.Load() != 0 {
					t.Errorf("invalid timeout submitted/polled runs: POSTs=%d GETs=%d", f.posts.Load(), f.polls.Load())
				}
			})
		}
	}
}
