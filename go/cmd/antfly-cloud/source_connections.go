package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
	"github.com/spf13/cobra"
)

func newSourceConnectionsCommand(stateFor stateFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "connections",
		Short: "Manage source connections",
		Long: `Manage source connections: the hosted credentials sources use to read Google
Workspace, Slack, Discord, or GCS content. Credentials stay with the hosted
service and are never downloaded by the CLI.

For an Antfly instance's own endpoint URLs, use ` + "`antfly-cloud connection`" + ` instead.`,
	}
	var provider string
	list := &cobra.Command{Use: "list", Short: "List stored source connections", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		items, err := st.client.SourceConnections(cmd.Context(), org.ID, instance.ID, provider)
		if err != nil {
			return err
		}
		return st.out.Print(items, func() error {
			rows := [][]string{}
			for _, item := range items {
				rows = append(rows, []string{item.ID, item.Provider, item.DisplayName, item.Status})
			}
			table(st.out.W, []string{"ID", "PROVIDER", "NAME", "STATUS"}, rows)
			return nil
		})
	}}
	list.Flags().StringVar(&provider, "provider", "", "provider filter")
	command.AddCommand(list)
	var services []string
	var reconnect string
	var noBrowser, noWait bool
	var name string
	connect := &cobra.Command{Use: "connect <google-workspace|slack|discord>", Short: "Authorize a source connection in your browser", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if name != "" && noWait {
			return &commands.Error{Code: commands.CodeInvalid, Message: "--name requires waiting for the connection; remove --no-wait"}
		}
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		provider := strings.ReplaceAll(args[0], "-", "_")
		result, err := st.client.StartSourceInstallation(cmd.Context(), org.ID, instance.ID, provider, reconnect, services)
		if err != nil {
			return err
		}
		if err := st.client.ValidateSourceInstallationURL(result.AuthorizationURL, provider, result.FlowID); err != nil {
			return err
		}
		target, _ := url.Parse(result.AuthorizationURL)

		if noWait {
			return st.out.PrintJSON(result)
		}
		fmt.Fprintf(st.out.W, "Approve %s access in your browser:\n%s\n", args[0], target.String())
		if !noBrowser {
			if err := openBrowser(target.String()); err != nil {
				fmt.Fprintln(st.out.W, "Open the URL above to continue.")
			}
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
		defer cancel()
		status, err := waitSourceInstallation(ctx, st.client, org.ID, instance.ID, provider, result.FlowID)
		if err != nil {
			return err
		}
		if name != "" {
			connection, err := st.client.RenameSourceConnection(cmd.Context(), org.ID, instance.ID, status.ProviderAuthID, name)
			if err != nil {
				return fmt.Errorf("connected %s but could not name it: %w", status.ProviderAuthID, err)
			}
			return st.out.Print(connection, func() error {
				fmt.Fprintf(st.out.W, "Connected: %s (%s)\nList content with: antfly-cloud connections resources %s --resource SERVICE\n", connection.DisplayName, connection.ID, connection.ID)
				return nil
			})
		}
		return st.out.Print(status, func() error {
			fmt.Fprintf(st.out.W, "Connected: %s\nList content with: antfly-cloud connections resources %s --resource SERVICE\n", status.ProviderAuthID, status.ProviderAuthID)
			return nil
		})
	}}
	connect.Flags().StringSliceVar(&services, "services", []string{"drive"}, "Google services: drive,gmail,calendar (incremental consent)")
	connect.Flags().StringVar(&reconnect, "reconnect", "", "existing connection ID to upgrade or reconnect")
	connect.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorization URL instead of opening a browser")
	connect.Flags().BoolVar(&noWait, "no-wait", false, "return the URL and flow ID without waiting")
	connect.Flags().StringVar(&name, "name", "", "display name for the new connection (usable in source manifests)")
	command.AddCommand(connect, newConnectionValidateCommand(stateFor), newConnectionRenameCommand(stateFor))
	var gcsBucket, gcsPrefix, gcsName string
	gcs := &cobra.Command{Use: "gcs", Short: "Register a scoped keyless GCS connection", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		connection, err := st.client.CreateGCSWorkloadConnection(cmd.Context(), org.ID, instance.ID, gcsName, gcsBucket, strings.TrimSuffix(gcsPrefix, "/"))
		if err != nil {
			return err
		}
		return st.out.Print(connection, func() error {
			fmt.Fprintf(st.out.W, "Registered GCS connection %s. Ask your Cloud operator to bind its import identity, then run antfly-cloud sources preview --connection %s\n", connection.ID, connection.ID)
			return nil
		})
	}}
	gcs.Flags().StringVar(&gcsBucket, "bucket", "", "GCS bucket")
	gcs.Flags().StringVar(&gcsPrefix, "prefix", "", "Allowed object prefix")
	gcs.Flags().StringVar(&gcsName, "name", "", "Connection display name")
	_ = gcs.MarkFlagRequired("bucket")
	_ = gcs.MarkFlagRequired("prefix")
	_ = gcs.MarkFlagRequired("name")
	add := &cobra.Command{Use: "add", Short: "Register a source connection"}
	add.AddCommand(gcs)
	command.AddCommand(add)

	var resource, parent, cursor string
	resources := &cobra.Command{Use: "resources <connection-id>", Short: "List selectable folders, Gmail labels, calendars, or channels", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		result, err := st.client.SourceResources(cmd.Context(), org.ID, instance.ID, args[0], resource, parent, cursor)
		if err != nil {
			return err
		}
		return st.out.Print(result, func() error {
			rows := [][]string{}
			for _, item := range result.Data {
				access := "readable"
				if !item.Readable {
					access = "needs access"
				}
				rows = append(rows, []string{item.ID, item.Name, access})
			}
			table(st.out.W, []string{"ID", "NAME", "ACCESS"}, rows)
			if result.NextCursor != "" {
				fmt.Fprintf(st.out.W, "Next cursor: %s\n", result.NextCursor)
			}
			return nil
		})
	}}
	resources.Flags().StringVar(&resource, "resource", "drive", "drive, gmail, calendar, slack, or discord")
	resources.Flags().StringVar(&parent, "parent", "", "Drive folder ID to browse")
	resources.Flags().StringVar(&cursor, "cursor", "", "next resource page cursor")
	command.AddCommand(resources)
	return command
}
func waitSourceInstallation(ctx context.Context, client *sdk.Client, org, instance, provider, flow string) (sdk.SourceInstallationStatus, error) {
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	for {
		status, err := client.SourceInstallationStatus(ctx, org, instance, provider, flow)
		if err != nil {
			return status, err
		}
		switch status.Status {
		case "connected":
			if status.ProviderAuthID == "" {
				return status, fmt.Errorf("installation response has no connection ID")
			}
			return status, nil
		case "denied", "failed":
			return status, fmt.Errorf("source authorization %s; existing connections were preserved", status.Status)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-timer.C:
		}
	}
}

func newSourcesCommand(stateFor stateFactory) *cobra.Command {
	command := &cobra.Command{Use: "sources", Short: "Manage connected sources, runs, and processing readiness"}
	addSourceManagementCommands(command, stateFor)
	command.AddCommand(newSourceManifestCommand("plan", stateFor), newSourceManifestCommand("apply", stateFor))
	for _, previewOnly := range []bool{true, false} {
		previewOnly := previewOnly
		var connection, resource, name, target string
		var selected []string
		var attachments, shared, pageProvenance bool
		var every, permissionTTL time.Duration
		use, short := "add", "Create a private source from selected content"
		if previewOnly {
			use = "preview"
			short = "Preview selected content before creating a source"
		}
		action := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if connection == "" {
				return fmt.Errorf("--connection is required")
			}
			if !previewOnly && (name == "" || target == "") {
				return fmt.Errorf("--name and --table are required")
			}
			st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
			if err != nil {
				return err
			}
			var chosen *sdk.SourceConnection
			if target != "" {
				resolved, err := st.client.ResolveSourceConnection(cmd.Context(), org.ID, instance.ID, connection, target)
				var apiErr *sdk.APIError
				switch {
				case err == nil:
					chosen = &resolved
				case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
					// Older Cloud releases only expose connection listing.
				default:
					return err
				}
			}
			if chosen == nil {
				connections, err := st.client.SourceConnections(cmd.Context(), org.ID, instance.ID, "")
				if err != nil {
					return err
				}
				for i := range connections {
					if connections[i].ID == connection {
						chosen = &connections[i]
						break
					}
				}
			}
			if chosen == nil || !chosen.Enabled || chosen.Status != "active" {
				return fmt.Errorf("source connection is not active")
			}
			if permissionTTL < time.Minute || permissionTTL > 24*time.Hour || permissionTTL%time.Second != 0 {
				return fmt.Errorf("--permission-ttl must be a whole number of seconds between 1m and 24h")
			}
			config := map[string]any{"attachments": attachments, "permission_ttl_seconds": int(permissionTTL / time.Second)}
			if chosen.Provider != "gcs" && (len(selected) == 0 || pageProvenance) {
				return fmt.Errorf("select content with --select; --page-provenance is only supported for GCS")
			}
			switch chosen.Provider {
			case "gcs":
				bucket, _ := chosen.Config["bucket"].(string)
				prefix, _ := chosen.Config["prefix"].(string)
				if bucket == "" || len(selected) > 1 {
					return fmt.Errorf("GCS requires a configured bucket and at most one --select prefix")
				}
				if len(selected) == 1 {
					prefix = selected[0]
				}
				delete(config, "attachments")
				config["bucket"] = bucket
				if prefix != "" {
					config["prefix"] = strings.TrimSuffix(prefix, "/") + "/"
				}
				if pageProvenance {
					config["publication"] = "text"
					config["page_provenance"] = true
					config["suffixes"] = []string{".txt"}
				}

			case "google_workspace":
				config["resource"] = resource
				switch resource {
				case "drive":
					if len(selected) != 1 {
						return fmt.Errorf("select one Drive folder per source")
					}
					config["folder_id"] = selected[0]
				case "gmail":
					config["label_ids"] = selected
				case "calendar":
					config["calendar_ids"] = selected
				default:
					return fmt.Errorf("choose --resource drive, gmail, or calendar")
				}
			case "slack":
				config["team_id"] = chosen.Config["team_id"]
				config["channel_ids"] = selected
			case "discord":
				config["guild_id"] = chosen.Config["guild_id"]
				config["channel_ids"] = selected
			default:
				return fmt.Errorf("unsupported source connection")
			}
			var preview sdk.SourcePreview
			if chosen.Provider == "gcs" {
				prefix, _ := config["prefix"].(string)
				preview, err = st.client.PreviewGCSSource(cmd.Context(), org.ID, instance.ID, connection, sdk.GCSSourceSelection{
					Prefix: prefix, PageProvenance: pageProvenance, TargetTable: target,
				})
			} else {
				preview, err = st.client.PreviewSource(cmd.Context(), org.ID, instance.ID, connection, config)
			}
			if err != nil {
				return err
			}
			if previewOnly {
				return st.out.PrintJSON(preview)
			}
			user, err := st.client.CurrentUser(cmd.Context())
			if err != nil {
				return err
			}
			readers := []string{}
			if !shared {
				readers = append(readers, "user:"+user.ID)
			}
			config["permissions"] = map[string]any{"public": shared, "readers": readers}
			seconds := int(every / time.Second)
			valid := false
			for _, allowed := range []int{300, 900, 1800, 3600, 21600, 43200, 86400} {
				if seconds == allowed {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("--every must be 5m, 15m, 30m, 1h, 6h, 12h, or 24h")
			}
			job, err := st.client.CreateSource(cmd.Context(), org.ID, instance.ID, sdk.CreateSourceRequest{Name: name, ProviderAuthID: connection, TargetTable: target, SourceConfig: config, RunPolicy: "scheduled", ScheduleSeconds: seconds})
			if err != nil {
				return err
			}
			if err := st.client.SyncSource(cmd.Context(), org.ID, instance.ID, job.ID); err != nil {
				return fmt.Errorf("source %s was saved; queue its first sync with `antfly-cloud sources sync %s`: %w", job.ID, job.ID, err)
			}
			return st.out.Print(job, func() error { fmt.Fprintf(st.out.W, "Source created and sync queued: %s\n", job.ID); return nil })
		}}
		action.Flags().StringVar(&connection, "connection", "", "source connection ID")
		action.Flags().StringVar(&resource, "resource", "drive", "Workspace service: drive, gmail, or calendar")
		action.Flags().StringSliceVar(&selected, "select", nil, "GCS object prefix or folder, label, calendar, or channel IDs")
		action.Flags().BoolVar(&pageProvenance, "page-provenance", false, "validate and preserve original PDF page citations on GCS text objects")
		action.Flags().BoolVar(&attachments, "attachments", false, "extract attachment contents")
		action.Flags().StringVar(&name, "name", "", "source name")
		action.Flags().StringVar(&target, "table", "", "destination table")
		action.Flags().DurationVar(&every, "every", 15*time.Minute, "content sync interval")
		action.Flags().DurationVar(&permissionTTL, "permission-ttl", 15*time.Minute, "maximum age of source access checks, independent of --every (1m..24h)")
		action.Flags().BoolVar(&shared, "shared", false, "share with all readers authorized for the destination table (default: only you)")
		command.AddCommand(action)
	}
	return command
}

func sourceCommandInstance(ctx context.Context, stateFor stateFactory) (*appState, Organization, CloudInstance, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	org, err := resolveOrg(ctx, st)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	if st.cfg.Instance == "" {
		return nil, Organization{}, CloudInstance{}, fmt.Errorf("select an instance with --instance or antfly-cloud instance use")
	}
	instance, err := resolveInstance(ctx, st, org.ID, st.cfg.Instance)
	return st, org, instance, err
}

func newConnectionValidateCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "validate <connection>", Short: "Check that a connection's hosted credentials still work", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, instance, id, err := sourceConnectionCommand(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		connection, err := st.client.ValidateSourceConnection(cmd.Context(), org.ID, instance.ID, id)
		if err != nil {
			return err
		}
		return st.out.Print(connection, func() error {
			_, err := fmt.Fprintf(st.out.W, "Connection %s (%s): %s\n", connection.DisplayName, connection.ID, connection.Status)
			return err
		})
	}}
}

func newConnectionRenameCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "rename <connection> <new-name>", Short: "Change a connection's display name", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, instance, id, err := sourceConnectionCommand(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		connection, err := st.client.RenameSourceConnection(cmd.Context(), org.ID, instance.ID, id, args[1])
		if err != nil {
			return err
		}
		return st.out.Print(connection, func() error {
			_, err := fmt.Fprintf(st.out.W, "Renamed connection %s to %s.\n", connection.ID, connection.DisplayName)
			return err
		})
	}}
}

// sourceConnectionCommand resolves the selected instance and a connection given
// by ID (used as-is, without listing) or unique display name.
func sourceConnectionCommand(ctx context.Context, stateFor stateFactory, ref string) (*appState, Organization, CloudInstance, string, error) {
	st, org, instance, err := sourceCommandInstance(ctx, stateFor)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, "", err
	}
	if isUUID(ref) {
		return st, org, instance, ref, nil
	}
	connections, err := listSourceConnectionsByName(ctx, st, org.ID, instance.ID)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, "", err
	}
	id, err := matchSourceConnectionName(connections, ref)
	return st, org, instance, id, err
}
