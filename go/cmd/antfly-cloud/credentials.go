package main

import (
	"errors"
	"fmt"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/spf13/cobra"
)

func newProviderAuthCommand(stateFor stateFactory) *cobra.Command {
	group := &cobra.Command{Use: "provider-auth", Short: "Manage instance-owned provider credentials (never prints secrets)"}
	group.AddCommand(newProviderAuthObjectsCommand(stateFor))
	group.AddCommand(&cobra.Command{Use: "list", Short: "List credential metadata", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ProviderAuths(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}})
	var file string
	create := &cobra.Command{Use: "create", Short: "Register a provider credential from JSON file or stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.CreateProviderAuthRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.CreateProviderAuth(cmd.Context(), org.ID, inst.ID, req)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	requestFileFlag(create, &file)
	group.AddCommand(create)
	group.AddCommand(&cobra.Command{Use: "delete <auth-id>", Short: "Delete an unreferenced credential (archived jobs still retain references)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		if err := st.client.DeleteProviderAuth(cmd.Context(), org.ID, inst.ID, args[0]); err != nil {
			return err
		}
		return st.out.PrintJSON(map[string]any{"id": args[0], "deleted": true})
	}})
	group.AddCommand(&cobra.Command{Use: "revoke <auth-id>", Short: "Disable a credential and permanently clear its stored secret, retaining job history", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.RevokeProviderAuth(cmd.Context(), org.ID, inst.ID, args[0])
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}})
	return group
}

func newInstanceAPIKeyCommand(stateFor stateFactory) *cobra.Command {
	group := &cobra.Command{Use: "api-key", Short: "Create and revoke instance data-plane API keys"}
	var file string
	create := &cobra.Command{Use: "create", Short: "Create an API key; prints the full secret exactly once", Long: "Create a data-plane API key from request JSON. Include key_type=read_only and explicit named-table grants for a table-scoped reader. Successful output intentionally includes the secret once; redirect it to a secure file. No secret is printed on error.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.CreateCloudAPIKeyRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.CreateCloudAPIKey(cmd.Context(), org.ID, inst.ID, req)
		if err != nil {
			return err
		}
		return st.out.Print(result, func() error {
			_, err := fmt.Fprintf(st.out.W, "ID: %s\nName: %s\nType: %s\nKey: %s\n", result.Id, result.Name, result.KeyType, result.Key)
			return err
		})
	}}
	requestFileFlag(create, &file)
	group.AddCommand(create)
	group.AddCommand(&cobra.Command{Use: "revoke <key-id>", Short: "Revoke an instance API key by ID", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		if err := st.client.RevokeCloudAPIKey(cmd.Context(), org.ID, inst.ID, args[0]); err != nil {
			return err
		}
		return st.out.PrintJSON(map[string]any{"id": args[0], "revoked": true})
	}})
	return group
}

func newProviderAuthObjectsCommand(stateFor stateFactory) *cobra.Command {
	var bucket, prefix string
	var limit int
	var recursive bool
	cmd := &cobra.Command{Use: "objects <auth-id>", Short: "List S3 object metadata for an authorized bucket", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if bucket == "" || limit < 1 || limit > 1000 {
			return errors.New("bucket is required and limit must be 1..1000")
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ProviderAuthObjects(cmd.Context(), org.ID, inst.ID, args[0], bucket, prefix, limit, recursive)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	cmd.Flags().StringVar(&bucket, "bucket", "", "authorized S3 bucket")
	_ = cmd.MarkFlagRequired("bucket")
	cmd.Flags().StringVar(&prefix, "prefix", "", "object key prefix")
	cmd.Flags().IntVar(&limit, "limit", 200, "maximum objects (1..1000)")
	cmd.Flags().BoolVar(&recursive, "recursive", false, "list nested objects rather than immediate prefixes")
	return cmd
}
