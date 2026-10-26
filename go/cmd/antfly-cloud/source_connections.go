package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	sdk "github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/spf13/cobra"
)

func newSourceConnectionsCommand(stateFor stateFactory) *cobra.Command {
	command := &cobra.Command{Use: "connections", Short: "Connect Google Workspace, Slack, and Discord sources"}
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
	connect := &cobra.Command{Use: "connect <google-workspace|slack|discord>", Short: "Authorize a source connection in your browser", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		return st.out.Print(status, func() error {
			fmt.Fprintf(st.out.W, "Connected: %s\nList content with: antfly-cloud connections resources %s --resource SERVICE\n", status.ProviderAuthID, status.ProviderAuthID)
			return nil
		})
	}}
	connect.Flags().StringSliceVar(&services, "services", []string{"drive"}, "Google services: drive,gmail,calendar (incremental consent)")
	connect.Flags().StringVar(&reconnect, "reconnect", "", "existing connection ID to upgrade or reconnect")
	connect.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorization URL instead of opening a browser")
	connect.Flags().BoolVar(&noWait, "no-wait", false, "return the URL and flow ID without waiting")
	command.AddCommand(connect)
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
	command := &cobra.Command{Use: "sources", Short: "Select, preview, and sync connected source content"}
	command.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		jobs, err := st.client.SourceJobs(cmd.Context(), org.ID, instance.ID)
		if err != nil {
			return err
		}
		return st.out.Print(jobs, func() error {
			rows := [][]string{}
			for _, job := range jobs {
				rows = append(rows, []string{job.ID, job.Name, job.Status})
			}
			table(st.out.W, []string{"ID", "SOURCE", "STATUS"}, rows)
			return nil
		})
	}})
	command.AddCommand(&cobra.Command{Use: "sync <source-id>", Args: cobra.ExactArgs(1), Short: "Queue a source sync", RunE: func(cmd *cobra.Command, args []string) error {
		st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
		if err != nil {
			return err
		}
		if err := st.client.SyncSource(cmd.Context(), org.ID, instance.ID, args[0]); err != nil {
			return err
		}
		fmt.Fprintln(st.out.W, "Sync queued.")
		return nil
	}})
	for _, previewOnly := range []bool{true, false} {
		previewOnly := previewOnly
		var connection, resource, name, target string
		var selected []string
		var attachments, shared bool
		var every time.Duration
		use, short := "add", "Create a private source from selected content"
		if previewOnly {
			use = "preview"
			short = "Preview selected content before creating a source"
		}
		action := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if connection == "" || len(selected) == 0 {
				return fmt.Errorf("--connection and --select are required")
			}
			if !previewOnly && (name == "" || target == "") {
				return fmt.Errorf("--name and --table are required")
			}
			st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
			if err != nil {
				return err
			}
			connections, err := st.client.SourceConnections(cmd.Context(), org.ID, instance.ID, "")
			if err != nil {
				return err
			}
			var chosen *sdk.SourceConnection
			for i := range connections {
				if connections[i].ID == connection {
					chosen = &connections[i]
					break
				}
			}
			if chosen == nil || !chosen.Enabled || chosen.Status != "active" {
				return fmt.Errorf("source connection is not active")
			}
			config := map[string]any{"attachments": attachments}
			switch chosen.Provider {
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
			preview, err := st.client.PreviewSource(cmd.Context(), org.ID, instance.ID, connection, config)
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
		action.Flags().StringSliceVar(&selected, "select", nil, "folder, label, calendar, or channel IDs")
		action.Flags().BoolVar(&attachments, "attachments", false, "extract attachment contents")
		action.Flags().StringVar(&name, "name", "", "source name")
		action.Flags().StringVar(&target, "table", "", "destination table")
		action.Flags().DurationVar(&every, "every", 15*time.Minute, "sync interval")
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
