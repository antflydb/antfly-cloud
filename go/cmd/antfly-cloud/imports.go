package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/spf13/cobra"
)

func importInstance(cmd *cobra.Command, stateFor stateFactory) (*appState, Organization, CloudInstance, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	org, err := resolveOrg(cmd.Context(), st)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	inst, err := resolveActiveInstance(cmd.Context(), st, org.ID, "")
	return st, org, inst, err
}

// Request parsing never includes user-controlled JSON values in errors: requests
// may contain S3 credentials. Stdin follows Cobra's reader for embedding/testing.
func readRequestFile(cmd *cobra.Command, path string, out any) error {
	reader := cmd.InOrStdin()
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return errors.New("cannot open request file")
		}
		defer file.Close()
		reader = file
	}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(out); err != nil {
		return errors.New("invalid request JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request file must contain exactly one JSON object")
	}
	return nil
}

func requestFileFlag(cmd *cobra.Command, file *string) {
	cmd.Flags().StringVar(file, "file", "", "request JSON file; use - for stdin")
	_ = cmd.MarkFlagRequired("file")
}

func newImportCommand(stateFor stateFactory) *cobra.Command {
	group := &cobra.Command{Use: "import", Short: "Import documents into an existing cloud instance", Long: "Manage document imports. Output is complete JSON (lists are arrays), including diagnostics. Select an existing instance with --instance or instance use."}
	group.AddCommand(&cobra.Command{Use: "capabilities", Short: "Show supported providers, formats, modes, and retryable errors", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ImportCapabilities(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}})
	var includeArchived bool
	list := &cobra.Command{Use: "list", Short: "List import jobs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ImportJobs(cmd.Context(), org.ID, inst.ID, includeArchived)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	list.Flags().BoolVar(&includeArchived, "include-archived", false, "include archived jobs")
	group.AddCommand(list)
	var file string
	create := &cobra.Command{Use: "create", Short: "Create an import job from request JSON", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.CreateImportJobRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.CreateImportJob(cmd.Context(), org.ID, inst.ID, req)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	requestFileFlag(create, &file)
	group.AddCommand(create)
	for _, action := range []string{"run", "retry-failed"} {
		group.AddCommand(&cobra.Command{Use: action + " <job-id>", Short: "Queue an import run (returns its specific run ID)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			st, org, inst, err := importInstance(cmd, stateFor)
			if err != nil {
				return err
			}
			var result *sdk.ImportJobRun
			if action == "run" {
				result, err = st.client.RunImportJob(cmd.Context(), org.ID, inst.ID, args[0])
			} else {
				result, err = st.client.RetryFailedImportFiles(cmd.Context(), org.ID, inst.ID, args[0])
			}
			if err != nil {
				return err
			}
			return st.out.PrintJSON(result)
		}})
	}
	var limit, offset int
	runs := &cobra.Command{Use: "runs <job-id>", Short: "List run history including failure diagnostics", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if limit < 1 || limit > 100 || offset < 0 {
			return errors.New("limit must be 1..100 and offset must be nonnegative")
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ImportJobRuns(cmd.Context(), org.ID, inst.ID, args[0], limit, offset)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	runs.Flags().IntVar(&limit, "limit", 20, "number of runs (1..100)")
	runs.Flags().IntVar(&offset, "offset", 0, "history offset")
	group.AddCommand(runs)
	group.AddCommand(&cobra.Command{Use: "health <job-id>", Short: "Show observed document and search coverage", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ImportJobHealth(cmd.Context(), org.ID, inst.ID, args[0])
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}})
	for _, action := range []string{"pause", "resume", "archive"} {
		group.AddCommand(&cobra.Command{Use: action + " <job-id>", Short: action + " an import job (retains history)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			st, org, inst, err := importInstance(cmd, stateFor)
			if err != nil {
				return err
			}
			var result *sdk.ImportJob
			switch action {
			case "pause":
				result, err = st.client.PauseImportJob(cmd.Context(), org.ID, inst.ID, args[0])
			case "resume":
				result, err = st.client.ResumeImportJob(cmd.Context(), org.ID, inst.ID, args[0])
			case "archive":
				result, err = st.client.ArchiveImportJob(cmd.Context(), org.ID, inst.ID, args[0])
			}
			if err != nil {
				return err
			}
			return st.out.PrintJSON(result)
		}})
	}
	var waitTimeout, pollInterval time.Duration
	wait := &cobra.Command{Use: "wait <job-id> <run-id>", Short: "Wait for this run; fail on partial, failed, or unknown outcome", Long: "Wait for the specified run, not the latest run. Timeout or Ctrl-C stops waiting without canceling the backend job. The last observed run and its diagnostics are printed even on failure.", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if waitTimeout <= 0 || pollInterval <= 0 {
			return errors.New("wait timeout and poll interval must be positive")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), waitTimeout)
		defer cancel()
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		run, waitErr := st.client.WaitImportJobRun(ctx, org.ID, inst.ID, args[0], args[1], pollInterval)
		if run != nil {
			if err := st.out.PrintJSON(run); err != nil {
				return err
			}
		}
		if waitErr != nil {
			return fmt.Errorf("waiting for import run %s: %w", args[1], waitErr)
		}
		return nil
	}}
	wait.Flags().DurationVar(&waitTimeout, "wait-timeout", 30*time.Minute, "overall wait timeout (independent of --timeout per API request)")
	wait.Flags().DurationVar(&pollInterval, "poll-interval", 2*time.Second, "time between run history polls")
	group.AddCommand(wait)
	return group
}
