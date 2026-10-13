package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/spf13/cobra"
)

func newConnectionsCommand(stateFor stateFactory) *cobra.Command {
	group := &cobra.Command{Use: "connections", Short: "Manage hosted instance connections (never prints secrets)", Long: "Connections live on the selected hosted instance. Local antfly connections login aws/google does not upload credentials to this vault."}
	group.AddCommand(newProviderAuthObjectsCommand(stateFor))
	group.AddCommand(newConnectionUpdateCommand(stateFor))
	group.AddCommand(&cobra.Command{Use: "get <connection-id>", Short: "Inspect hosted connection metadata", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.GetProviderAuth(cmd.Context(), org.ID, inst.ID, args[0])
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}})
	group.AddCommand(&cobra.Command{Use: "validate <connection-id>", Short: "Validate stored provider credentials; does not prove runtime adoption", Long: "Validate stored provider credentials, including recovery after failed replacement validation. Success does not prove asynchronous runtime reconciliation or uninterrupted active imports. Rotate outside active imports.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.ValidateProviderAuth(cmd.Context(), org.ID, inst.ID, args[0])
		if err != nil {
			return err
		}
		if err := st.out.PrintJSON(result); err != nil {
			return err
		}
		return connectionValidationResult(result)
	}})
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
	var file, provider string
	create := &cobra.Command{Use: "create", Short: "Register a provider credential from JSON file or stdin", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.CreateProviderAuthRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		if err := requestSelector(cmd, "provider", provider, &req.Provider); err != nil {
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
	create.Flags().StringVar(&provider, "provider", "", "provider; supplies the JSON provider field")
	group.AddCommand(create)
	group.AddCommand(&cobra.Command{Use: "delete <connection-id>", Short: "Delete only unreferenced metadata (archived jobs retain references)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		if err := st.client.DeleteProviderAuth(cmd.Context(), org.ID, inst.ID, args[0]); err != nil {
			return err
		}
		return st.out.PrintJSON(map[string]any{"id": args[0], "deleted": true})
	}})
	group.AddCommand(&cobra.Command{Use: "revoke <connection-id>", Short: "Disable and clear the hosted secret; does not revoke the upstream provider key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	create := &cobra.Command{Use: "create", Short: "Create an API key; prints the full secret exactly once", Long: "Create a data-plane API key from request JSON. Include key_type=read_only and explicit named-table grants for a table-scoped reader. Omitted or empty grants create an instance-wide key; use table_name=* for an explicit all-table grant. Successful output intentionally includes the secret once; redirect it to a secure file. No secret is printed on error.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.CreateCloudAPIKeyRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		for _, grant := range req.Grants {
			if strings.TrimSpace(grant.TableName) == "" {
				return errors.New("each grant requires an explicit table_name; use * for all tables")
			}
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

func connectionValidationResult(result *sdk.ProviderAuth) error {
	if result.Status != "active" {
		return errors.New("connection validation did not return active status")
	}
	return nil
}

func validateConnectionUpdate(req sdk.UpdateProviderAuthRequest) error {
	if req.DisplayName == nil && req.Secret == nil && req.ReplaceSecret == nil && req.ClearSecret == nil && req.Config == nil && req.Enabled == nil && req.IsDefault == nil {
		return errors.New("connection update requires at least one field")
	}
	replace := req.ReplaceSecret != nil && *req.ReplaceSecret
	if req.Secret != nil && (!replace || strings.TrimSpace(*req.Secret) == "") {
		return errors.New("secret replacement requires replace_secret=true and a nonblank secret")
	}
	if replace && req.Secret == nil {
		return errors.New("replace_secret=true requires a secret")
	}
	if req.ClearSecret != nil && *req.ClearSecret && replace {
		return errors.New("clear_secret cannot be combined with secret replacement")
	}
	return nil
}

func newConnectionUpdateCommand(stateFor stateFactory) *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "update <connection-id>", Short: "Update metadata or replace stored credentials and validate them", Long: "Update a hosted connection from JSON. Secret replacement requires replace_secret=true and automatically validates the stored replacement. Failed validation retains the connection ID and prints PATCH metadata; use connections validate to recover. Metadata or clear-only updates do not validate. Provider validation does not prove asynchronous runtime adoption; rotate outside active imports.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var req sdk.UpdateProviderAuthRequest
		if err := readRequestFile(cmd, file, &req); err != nil {
			return err
		}
		if err := validateConnectionUpdate(req); err != nil {
			return err
		}
		st, org, inst, err := importInstance(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.UpdateProviderAuth(cmd.Context(), org.ID, inst.ID, args[0], req)
		if err != nil {
			return err
		}
		if req.ReplaceSecret != nil && *req.ReplaceSecret {
			validated, validationErr := st.client.ValidateProviderAuth(cmd.Context(), org.ID, inst.ID, args[0])
			if validationErr == nil {
				validationErr = connectionValidationResult(validated)
			}
			if validationErr != nil {
				if err := st.out.PrintJSON(result); err != nil {
					return err
				}
				return validationErr
			}
			result = validated
		}
		return st.out.PrintJSON(result)
	}}
	requestFileFlag(cmd, &file)
	return cmd
}
