package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/antflydb/antfly/go/pkg/sources/management"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
	"github.com/spf13/cobra"
)

// sourceManagementEndpoint scopes the management mount to an organization and instance.
func sourceManagementEndpoint(apiURL, orgID, instanceID string) string {
	return strings.TrimRight(apiURL, "/") + "/organizations/" + url.PathEscape(orgID) +
		"/cloud/instances/" + url.PathEscape(instanceID) + "/sources/v1"
}

// sourceManagement resolves the selected scope and refreshes credentials per request.
func sourceManagement(cmd *cobra.Command, stateFor stateFactory) (commands.Env, error) {
	st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
	if err != nil {
		return commands.Env{}, err
	}
	return sourceManagementForInstance(stateFor, st, org.ID, instance.ID)
}

// sourceManagementForInstance builds the management environment for an already resolved scope.
func sourceManagementForInstance(stateFor stateFactory, st *appState, orgID, instanceID string) (commands.Env, error) {
	client, err := management.NewClientWithOptions(sourceManagementEndpoint(st.cfg.APIURL, orgID, instanceID), management.Options{
		HTTPClient: &http.Client{Timeout: st.timeout, Transport: userAgentTransport{http.DefaultTransport}},
		Token: func(context.Context) (string, error) {
			current, err := stateFor(true)
			if err != nil {
				return "", err
			}
			return current.cfg.bearerToken(), nil
		},
	})
	if err != nil {
		return commands.Env{}, err
	}
	return commands.Env{API: client, Out: st.out.W, JSON: st.out.JSON}, nil
}

// addSourceManagementCommands wires the shared source management commands into the sources group.
func addSourceManagementCommands(command *cobra.Command, stateFor stateFactory) {
	command.AddCommand(&cobra.Command{Use: "capabilities", Short: "Show source management capabilities", Args: cobra.NoArgs,
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, _ []string) error {
			_, err := commands.Capabilities(cmd.Context(), env)
			return err
		})})

	var opts commands.ListOptions
	list := &cobra.Command{Use: "list", Short: "List sources for the selected instance", Args: cobra.NoArgs,
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, _ []string) error {
			_, err := commands.ListSources(cmd.Context(), env, opts)
			return err
		})}
	list.Flags().IntVar(&opts.Limit, "limit", 20, "maximum sources per page")
	list.Flags().StringVar(&opts.Cursor, "cursor", "", "next source page cursor")
	list.Flags().BoolVar(&opts.All, "all", false, "list all source pages")
	command.AddCommand(list)

	command.AddCommand(&cobra.Command{Use: "status <source>", Aliases: []string{"get"}, Short: "Show acquisition and search readiness by source ID or unique name", Args: cobra.ExactArgs(1),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.Status(cmd.Context(), env, args[0])
			return err
		})})

	command.AddCommand(newSourceRunCommand("run", stateFor))
	sync := newSourceRunCommand("sync", stateFor)
	sync.Deprecated = "use `antfly-cloud sources run`"
	command.AddCommand(sync, newSourceRunsCommand(stateFor))

	command.AddCommand(&cobra.Command{
		Use:   "archive <source>",
		Short: "Archive a source while retaining indexed content",
		Long:  "Archive a source. Archive retains indexed content; purge deletes only source-owned records. Removing an entry from a manifest does neither.",
		Args:  cobra.ExactArgs(1),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.Archive(cmd.Context(), env, args[0])
			return err
		}),
	})
	command.AddCommand(newSourcePurgeCommand(stateFor))

	processing := &cobra.Command{Use: "processing", Short: "Inspect source processing readiness"}
	processing.AddCommand(&cobra.Command{Use: "status <source>", Short: "Show processing readiness separately from acquisition runs", Args: cobra.ExactArgs(1),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.ProcessingStatus(cmd.Context(), env, args[0])
			return err
		})})
	artifacts := &cobra.Command{Use: "artifacts", Short: "Inspect source artifacts"}
	artifacts.AddCommand(&cobra.Command{Use: "list <source>", Short: "List source artifacts and their readiness", Args: cobra.ExactArgs(1),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.ListArtifacts(cmd.Context(), env, args[0])
			return err
		})})
	command.AddCommand(processing, artifacts, newSourceScheduleCommand("pause", stateFor), newSourceScheduleCommand("resume", stateFor))
}

// withSourceManagement resolves the management environment before running a command.
func withSourceManagement(stateFor stateFactory, run func(cmd *cobra.Command, env commands.Env, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		env, err := sourceManagement(cmd, stateFor)
		if err != nil {
			return err
		}
		return run(cmd, env, args)
	}
}

func newSourceScheduleCommand(action string, stateFor stateFactory) *cobra.Command {
	verb, receipt := "Pause", "Paused"
	if action == "resume" {
		verb, receipt = "Resume", "Resumed"
	}
	return &cobra.Command{
		Use:   action + " <source>",
		Short: verb + " the hosted schedule, not a running acquisition",
		Long:  verb + " the hosted schedule of a connected source. This Cloud-only action does not change a running acquisition.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
			if err != nil {
				return err
			}
			env, err := sourceManagementForInstance(stateFor, st, org.ID, instance.ID)
			if err != nil {
				return err
			}
			env.Out = io.Discard
			source, err := commands.ResolveSource(cmd.Context(), env, args[0])
			if err != nil {
				return err
			}
			schedule := st.client.PauseSource
			if action == "resume" {
				schedule = st.client.ResumeSource
			}
			job, err := schedule(cmd.Context(), org.ID, instance.ID, source.Id)
			if err != nil {
				return err
			}
			return st.out.Print(job, func() error {
				_, err := fmt.Fprintf(st.out.W, "%s source %s (%s).\n", receipt, source.Name, source.Id)
				return err
			})
		},
	}
}

func newSourceRunCommand(name string, stateFor stateFactory) *cobra.Command {
	var opts commands.RunOptions
	var wait bool
	var waitTimeout time.Duration
	command := &cobra.Command{
		Use:   name + " <source>",
		Short: "Start a source acquisition run",
		Args:  cobra.ExactArgs(1),
		Long:  "Start a source acquisition run. With --wait, watch acquisition progress without cancelling the hosted run on timeout or interruption. With --wait --json, print one receipt containing the final or last-observed run and its idempotency key, even if watching fails.",
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}
			receipt, err := commands.StartRun(cmd.Context(), runStartEnv(env, wait), args[0], opts)
			if err != nil || !wait {
				return err
			}
			return waitAndPrintReceipt(cmd, env, &receipt, &receipt.Run, waitTimeout)
		}),
	}
	command.Flags().BoolVar(&wait, "wait", false, "watch the run until acquisition finishes")
	addWaitTimeoutFlag(command, &waitTimeout)
	command.Flags().BoolVar(&opts.Restart, "restart", false, "restart source acquisition")
	command.Flags().StringVar(&opts.IdempotencyKey, "idempotency-key", "", "key to safely retry the same submission")
	return command
}

func newSourcePurgeCommand(stateFor stateFactory) *cobra.Command {
	var opts commands.PurgeOptions
	var wait bool
	var waitTimeout time.Duration
	command := &cobra.Command{
		Use:   "purge <source>",
		Short: "Purge source-owned indexed records with explicit confirmation",
		Long: "Purge deletes only source-owned records. Archive retains indexed content. Removing an entry from a manifest does neither. " +
			"Requires --yes. With --wait, watch the purge run without cancelling it on timeout or interruption. " +
			"With --wait --json, print one receipt containing the mutation, final or last-observed run, and idempotency key, even if watching fails.",
		Args: cobra.ExactArgs(1),
		// Refuse before resolving credentials or scope, so an unconfirmed purge
		// never depends on login state. Purge itself rejects without API calls.
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Confirm {
				return nil
			}
			_, err := commands.Purge(cmd.Context(), commands.Env{}, args[0], opts)
			return err
		},
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}
			receipt, err := commands.Purge(cmd.Context(), runStartEnv(env, wait), args[0], opts)
			if err != nil || !wait {
				return err
			}
			return waitAndPrintReceipt(cmd, env, &receipt, &receipt.Run, waitTimeout)
		}),
	}
	command.Flags().BoolVar(&opts.Confirm, "yes", false, "confirm deletion of source-owned indexed records")
	command.Flags().StringVar(&opts.IdempotencyKey, "idempotency-key", "", "key to safely retry the same submission")
	command.Flags().BoolVar(&wait, "wait", false, "watch the purge run until it finishes")
	addWaitTimeoutFlag(command, &waitTimeout)
	return command
}

func newSourceRunsCommand(stateFor stateFactory) *cobra.Command {
	command := &cobra.Command{Use: "runs", Short: "List, inspect, watch, or cancel source acquisition runs"}
	var opts commands.ListOptions
	list := &cobra.Command{Use: "list <source>", Short: "List source acquisition runs", Args: cobra.ExactArgs(1),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.ListRuns(cmd.Context(), env, args[0], opts)
			return err
		})}
	list.Flags().IntVar(&opts.Limit, "limit", 20, "maximum runs per page")
	list.Flags().StringVar(&opts.Cursor, "cursor", "", "next run page cursor")
	list.Flags().BoolVar(&opts.All, "all", false, "list all run pages")
	command.AddCommand(list)

	command.AddCommand(&cobra.Command{Use: "inspect <source> <run-id>", Short: "Show acquisition run details", Args: cobra.ExactArgs(2),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.InspectRun(cmd.Context(), env, args[0], args[1])
			return err
		})})

	var waitTimeout time.Duration
	wait := &cobra.Command{Use: "wait <source> <run-id>", Short: "Watch an existing acquisition run", Args: cobra.ExactArgs(2),
		Long: "Watch an existing acquisition run. Timeout or interruption stops watching without cancelling the hosted run. JSON output is a single bare run object.",
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			if err := validateWaitTimeout(waitTimeout); err != nil {
				return err
			}
			silent := env
			silent.Out = io.Discard
			source, err := commands.ResolveSource(cmd.Context(), silent, args[0])
			if err != nil {
				return err
			}
			run, err := waitSourceRun(cmd, env, source.Id, args[1], waitTimeout)
			return printWaitResult(env, run, err)
		})}
	addWaitTimeoutFlag(wait, &waitTimeout)
	command.AddCommand(wait)

	command.AddCommand(&cobra.Command{Use: "cancel <source> <run-id>", Short: "Request cancellation of an acquisition run", Args: cobra.ExactArgs(2),
		RunE: withSourceManagement(stateFor, func(cmd *cobra.Command, env commands.Env, args []string) error {
			_, err := commands.CancelRun(cmd.Context(), env, args[0], args[1])
			return err
		})})
	return command
}

func addWaitTimeoutFlag(command *cobra.Command, timeout *time.Duration) {
	command.Flags().DurationVar(timeout, "wait-timeout", 30*time.Minute, "maximum time to watch the run")
}

func validateWaitTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return &commands.Error{Code: commands.CodeInvalid, Message: "--wait-timeout must be greater than 0"}
	}
	return nil
}

// runStartEnv silences the start command's output under --wait --json so the
// final receipt is the only JSON printed.
func runStartEnv(env commands.Env, wait bool) commands.Env {
	if wait && env.JSON {
		env.Out = io.Discard
	}
	return env
}

// waitAndPrintReceipt watches a started run, records the final or last-observed
// run in the receipt, and prints the receipt. run must point into receipt.
func waitAndPrintReceipt(cmd *cobra.Command, env commands.Env, receipt any, run *management.Run, timeout time.Duration) error {
	observed, err := waitSourceRun(cmd, env, run.SourceId, run.Id, timeout)
	if observed.Id != "" {
		*run = observed
	}
	return printWaitResult(env, receipt, err)
}

// printWaitResult renders the final JSON result of a watch even when watching
// failed; a write failure is reported only if watching itself succeeded.
func printWaitResult(env commands.Env, result any, waitErr error) error {
	if !env.JSON {
		return waitErr
	}
	writeErr := (Output{W: env.Out}).PrintJSON(result)
	if waitErr != nil {
		return waitErr
	}
	return commands.Classify(writeErr)
}

// waitSourceRun leaves final JSON rendering to the caller so receipts stay atomic.
func waitSourceRun(cmd *cobra.Command, env commands.Env, sourceID, runID string, timeout time.Duration) (management.Run, error) {
	waitEnv := env
	if env.JSON {
		waitEnv.Out = io.Discard
	}
	run, err := commands.Wait(cmd.Context(), waitEnv, sourceID, runID, commands.WaitOptions{Timeout: timeout})
	var failure *commands.Error
	if !env.JSON && errors.As(err, &failure) && (failure.Code == commands.CodeTimeout || failure.Code == commands.CodeInterrupted) {
		fmt.Fprintf(env.Out, "Resume watching with: antfly-cloud sources runs wait %s %s\n", sourceID, runID)
	}
	return run, err
}

// userAgentTransport identifies shared-management requests as the Cloud CLI,
// matching the Cloud SDK's source requests.
type userAgentTransport struct{ base http.RoundTripper }

func (t userAgentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", sdk.UserAgent)
	return t.base.RoundTrip(r)
}
