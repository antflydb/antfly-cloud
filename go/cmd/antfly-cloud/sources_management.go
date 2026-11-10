package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"

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
func sourceManagement(cmd *cobra.Command, stateFor stateFactory) (commands.Env, *appState, error) {
	st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
	if err != nil {
		return commands.Env{}, st, err
	}
	client, err := management.NewClientWithOptions(sourceManagementEndpoint(st.cfg.APIURL, org.ID, instance.ID), management.Options{
		HTTPClient: &http.Client{Timeout: st.timeout},
		Token: func(context.Context) (string, error) {
			current, err := stateFor(true)
			if err != nil {
				return "", err
			}
			return current.cfg.bearerToken(), nil
		},
	})
	if err != nil {
		return commands.Env{}, st, err
	}
	return commands.Env{API: client, Out: st.out.W, JSON: st.out.JSON}, st, nil
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
}

// withSourceManagement resolves the management environment before running a command.
func withSourceManagement(stateFor stateFactory, run func(cmd *cobra.Command, env commands.Env, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		env, _, err := sourceManagement(cmd, stateFor)
		if err != nil {
			return err
		}
		return run(cmd, env, args)
	}
}
