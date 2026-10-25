package main

import (
	"fmt"
	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/antflydb/antfly-cloud/go/pkg/sdk/oapi"
	"github.com/spf13/cobra"
)

func newLiteCommand(stateFor stateFactory) *cobra.Command {
	root := &cobra.Command{Use: "lite", Short: "Create, import and take portable Lite databases home"}
	printOperation := func(st *appState, op *sdk.LiteOperation) error {
		return st.out.Print(op, func() error {
			fmt.Fprintf(st.out.W, "Operation %s: %s\n", op.Id, op.Phase)
			if op.InstanceId.String() != "00000000-0000-0000-0000-000000000000" {
				fmt.Fprintf(st.out.W, "Database: %s\n", op.InstanceId)
			}
			return nil
		})
	}
	root.AddCommand(&cobra.Command{Use: "capabilities", Short: "Show the server's Lite availability for this organization", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		caps, err := st.client.Capabilities(cmd.Context(), org.ID)
		if err != nil {
			return err
		}
		return st.out.Print(caps, func() error {
			fmt.Fprintf(st.out.W, "Create: %t  Import: %t  Export: %t\n", caps.CreateLite, caps.ImportLite, caps.ExportLite)
			return nil
		})
	}})
	var name, slug, region, storage string
	var acceptTerms, claimIntro bool
	create := &cobra.Command{Use: "create", Short: "Create a new empty managed Lite database", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		if name == "" || slug == "" {
			return fmt.Errorf("--name and --slug are required")
		}
		instance, err := st.client.CreateInstance(cmd.Context(), org.ID, liteCreateRequest(name, slug, region, storage, acceptTerms, claimIntro))
		if err != nil {
			return err
		}
		return st.out.Print(instance, func() error { printInstance(st.out.W, *instance); return nil })
	}}
	create.Flags().StringVar(&name, "name", "", "Database display name")
	create.Flags().StringVar(&slug, "slug", "", "New database slug")
	create.Flags().StringVar(&region, "region", "us-central1", "Database region")
	create.Flags().StringVar(&storage, "storage", "50Gi", "Lite includes 50 GiB persistent disk")
	create.Flags().BoolVar(&acceptTerms, "accept-lite-terms", false, "Accept USD 20/month after any free year, including while asleep with retained storage")
	create.Flags().BoolVar(&claimIntro, "claim-free-year", false, "Claim the limited one-database free year for your organization; fail if unavailable")
	root.AddCommand(create)
	var key string
	var wait bool
	importCmd := &cobra.Command{Use: "import <snapshot.aflite>", Short: "Upload an independent snapshot and inspect it before creating a database", Long: "First run: antfly lite snapshot local.aflite --out upload.aflite. For live application cutover, pause writes before taking that snapshot. Later local edits remain local. Reuse --idempotency-key and the same snapshot to resume an interrupted upload.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if key == "" {
			return fmt.Errorf("--idempotency-key is required and must be reused when resuming")
		}
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		op, err := st.client.UploadLiteSnapshot(cmd.Context(), org.ID, args[0], key, sdk.TransferOptions{})
		if err != nil {
			if op != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Resume operation %s with the same snapshot and idempotency key.\n", op.Id)
			}
			return err
		}
		if wait {
			op, err = st.client.WaitLiteOperation(cmd.Context(), org.ID, op.Id.String())
			if err != nil {
				if op != nil {
					_ = printOperation(st, op)
				}
				return err
			}
		}
		return printOperation(st, op)
	}}
	importCmd.Flags().StringVar(&key, "idempotency-key", "", "Stable unique key for this snapshot upload")
	importCmd.Flags().BoolVar(&wait, "wait", true, "Wait for verified preflight or failure")
	root.AddCommand(importCmd)
	root.AddCommand(&cobra.Command{Use: "status <operation-id>", Short: "Inspect status and dependency report", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		op, err := st.client.LiteOperation(cmd.Context(), org.ID, args[0])
		if err != nil {
			return err
		}
		return st.out.Print(op, func() error {
			printOperation(st, op)
			if op.Result != nil {
				fmt.Fprintf(st.out.W, "Report: %v\n", op.Result)
			}
			return nil
		})
	}})
	root.AddCommand(&cobra.Command{Use: "cancel <operation-id>", Short: "Cancel an unconfirmed import or pending export; cleanup is asynchronous", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		op, err := st.client.CancelLiteOperation(cmd.Context(), org.ID, args[0])
		if err != nil {
			return err
		}
		return printOperation(st, op)
	}})
	var confirmName, confirmSlug, confirmRegion, confirmStorage string
	var revision int64
	var acknowledged, confirmTerms, confirmIntro bool
	confirm := &cobra.Command{Use: "confirm <operation-id>", Short: "Create a new database from a reviewed import report", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !acknowledged || revision <= 0 || confirmName == "" || confirmSlug == "" {
			return fmt.Errorf("provide --revision from the reviewed report, --name, --slug and --acknowledge-snapshot-boundary")
		}
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		instance, err := st.client.ConfirmLiteImport(cmd.Context(), org.ID, args[0], sdk.ConfirmLiteImportRequest{Revision: revision, SnapshotBoundaryAcknowledged: true, Instance: liteCreateRequest(confirmName, confirmSlug, confirmRegion, confirmStorage, confirmTerms, confirmIntro)})
		if err != nil {
			return err
		}
		return st.out.Print(instance, func() error { printInstance(st.out.W, *instance); return nil })
	}}
	confirm.Flags().StringVar(&confirmName, "name", "", "New database name")
	confirm.Flags().StringVar(&confirmSlug, "slug", "", "New database slug")
	confirm.Flags().StringVar(&confirmRegion, "region", "us-central1", "Database region")
	confirm.Flags().StringVar(&confirmStorage, "storage", "50Gi", "Capacity for source plus independent snapshot and 1 GiB headroom")
	confirm.Flags().Int64Var(&revision, "revision", 0, "Revision from the reviewed awaiting_confirmation report")
	confirm.Flags().BoolVar(&acknowledged, "acknowledge-snapshot-boundary", false, "Acknowledge independent copy, excluded later writes and explicit application write-pause boundary")
	confirm.Flags().BoolVar(&confirmTerms, "accept-lite-terms", false, "Accept USD 20/month after any free year, including while asleep with retained storage")
	confirm.Flags().BoolVar(&confirmIntro, "claim-free-year", false, "Claim the limited organization free year; fail if unavailable")
	root.AddCommand(confirm)
	var exportKey string
	var exportWait bool
	export := &cobra.Command{Use: "export <instance-id-or-slug>", Short: "Create a stable Lite snapshot from the managed database", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if exportKey == "" {
			return fmt.Errorf("--idempotency-key is required")
		}
		st, org, instance, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		op, err := st.client.StartLiteExport(cmd.Context(), org.ID, instance.ID, exportKey)
		if err != nil {
			return err
		}
		if exportWait {
			op, err = st.client.WaitLiteOperation(cmd.Context(), org.ID, op.Id.String())
			if err != nil {
				return err
			}
		}
		return printOperation(st, op)
	}}
	export.Flags().StringVar(&exportKey, "idempotency-key", "", "Stable key; retries keep the first verified snapshot")
	export.Flags().BoolVar(&exportWait, "wait", true, "Wait until the snapshot is downloadable")
	root.AddCommand(export)
	root.AddCommand(&cobra.Command{Use: "download <operation-id> <new.aflite>", Short: "Resume and verify an export into a new local file", Long: "Downloads a pinned export with bounded I/O and SHA-256 verification. Existing destinations are never replaced. Before using query embedding or new ingestion, run Antfly preflight against your local model/media capabilities; remote dependencies require explicit configuration.", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		if err := st.client.DownloadLiteFile(cmd.Context(), org.ID, args[0], args[1], sdk.TransferOptions{}); err != nil {
			return err
		}
		return st.out.Print(map[string]string{"path": args[1], "operation_id": args[0]}, func() error { fmt.Fprintf(st.out.W, "Verified Lite file: %s\n", args[1]); return nil })
	}})
	root.AddCommand(&cobra.Command{Use: "resume <instance-id-or-slug>", Short: "Resume the same Lite database on its retained disk; wait for ready before querying", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, instance, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		result, err := st.client.ResumeLite(cmd.Context(), org.ID, instance.ID)
		if err != nil {
			return err
		}
		return st.out.Print(result, func() error { printInstance(st.out.W, *result); return nil })
	}})
	return root
}
func liteCreateRequest(name, slug, region, storage string, accept, intro bool) sdk.CreateInstanceRequest {
	return sdk.CreateInstanceRequest{AcceptLiteTerms: accept, ClaimLiteIntro: intro, Name: name, Slug: slug, Region: region, Mode: "single", Tier: "lite", DeploymentProfile: "lite", VersionPolicy: "pinned", NodeConfig: oapi.NodeConfigUpdate{DataStorage: storage}}
}
