package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newBetaCommand(stateFor stateFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use: "beta", Short: "Inspect opt-in Antfly Cloud beta features",
		Long: "Read-only beta commands for enrolled organizations. Availability is controlled by the server. These commands do not connect accounts, share content, or start syncs.",
	}
	cmd.AddCommand(&cobra.Command{
		Use: "capabilities", Short: "Show connected-source beta capabilities", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, org, instance, err := accessInstance(cmd, stateFor, nil)
			if err != nil {
				return err
			}
			capabilities, err := st.client.BetaConnectedSourceCapabilities(cmd.Context(), org.ID, instance.ID)
			if err != nil {
				return connectedSourcesBetaError(err)
			}
			return st.out.Print(capabilities, func() error {
				fmt.Fprintf(st.out.W, "Connected sources (Beta) — %s / %s\n", org.Name, instance.Name)
				fmt.Fprintf(st.out.W, "Available operations: %s\nConnection providers: %s\nSource management: %t\nSearch: %t\n",
					strings.Join(capabilities.Operations, ", "), strings.Join(capabilities.ConnectionProviders, ", "), capabilities.SourceManagement, capabilities.Search)
				return nil
			})
		},
	})
	connections := &cobra.Command{Use: "connections", Short: "Inspect saved source connections (Beta)"}
	connections.AddCommand(&cobra.Command{
		Use: "list", Short: "List saved connection summaries without credentials", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, org, instance, err := accessInstance(cmd, stateFor, nil)
			if err != nil {
				return err
			}
			items, err := st.client.BetaSourceConnections(cmd.Context(), org.ID, instance.ID)
			if err != nil {
				return connectedSourcesBetaError(err)
			}
			return st.out.Print(items, func() error {
				fmt.Fprintf(st.out.W, "Source connections (Beta) — %s / %s\n", org.Name, instance.Name)
				if len(items) == 0 {
					fmt.Fprintln(st.out.W, "No supported saved connections found.")
					return nil
				}
				rows := make([][]string, 0, len(items))
				for _, item := range items {
					rows = append(rows, []string{item.ID, item.Provider, item.DisplayName, strconv.FormatBool(item.Enabled), item.Status})
				}
				table(st.out.W, []string{"ID", "PROVIDER", "NAME", "ENABLED", "SAVED STATUS"}, rows)
				return nil
			})
		},
	})
	cmd.AddCommand(connections)
	return cmd
}

func connectedSourcesBetaError(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return fmt.Errorf("connected sources beta is unavailable for the selected organization/instance or server version: %w", err)
	}
	return err
}
