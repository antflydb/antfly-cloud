package main

import (
	"encoding/json"
	"net/http"

	"github.com/spf13/cobra"
)

func dataPlaneClient(cmd *cobra.Command, stateFor stateFactory) (*appState, *Client, error) {
	st, conn, err := antflyContextForCommand(cmd, stateFor, nil)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewClient(conn.URL, conn.Token, &http.Client{Timeout: st.timeout})
	return st, client, err
}

func newInstanceQueryCommand(stateFor stateFactory) *cobra.Command {
	var file, table string
	cmd := &cobra.Command{Use: "query", Short: "Query a named instance table using a JSON request", Long: "Send query JSON to the named-table data-plane endpoint using the existing Cloud OAuth connection. Supports full_text_index and hierarchy without modifying the request. Management API keys cannot access this endpoint.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var request json.RawMessage
		if err := readRequestFile(cmd, file, &request); err != nil {
			return err
		}
		st, client, err := dataPlaneClient(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := client.QueryTable(cmd.Context(), table, request)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	requestFileFlag(cmd, &file)
	cmd.Flags().StringVar(&table, "table", "", "explicit table name")
	_ = cmd.MarkFlagRequired("table")
	return cmd
}

func newInstanceTableCommand(stateFor stateFactory) *cobra.Command {
	group := &cobra.Command{Use: "table", Short: "Inspect or drop a named instance table"}
	var getTable, dropTable string
	get := &cobra.Command{Use: "get", Short: "Show table schema, indexes, and configuration", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, client, err := dataPlaneClient(cmd, stateFor)
		if err != nil {
			return err
		}
		result, err := client.GetTable(cmd.Context(), getTable)
		if err != nil {
			return err
		}
		return st.out.PrintJSON(result)
	}}
	get.Flags().StringVar(&getTable, "table", "", "explicit table name")
	_ = get.MarkFlagRequired("table")
	group.AddCommand(get)
	drop := &cobra.Command{Use: "drop", Short: "Permanently drop this explicit table and its documents", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		st, client, err := dataPlaneClient(cmd, stateFor)
		if err != nil {
			return err
		}
		if err := client.DropTable(cmd.Context(), dropTable); err != nil {
			return err
		}
		return st.out.PrintJSON(map[string]any{"table_name": dropTable, "dropped": true})
	}}
	drop.Flags().StringVar(&dropTable, "table", "", "explicit table name")
	_ = drop.MarkFlagRequired("table")
	group.AddCommand(drop)
	return group
}
