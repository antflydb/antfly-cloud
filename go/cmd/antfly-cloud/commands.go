package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

type stateFactory func(requireToken bool) (*appState, error)

func newLoginCommand(stdout io.Writer, stateFor stateFactory, configFile *string) *cobra.Command {
	var tokenFlag, oidcIssuer, oidcClientID string
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Antfly Cloud",
		Long:  "Log in with the Antfly Cloud device flow, or store a management token with --token.",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := stateFor(false)
			if err != nil {
				return err
			}

			if strings.TrimSpace(tokenFlag) != "" {
				return loginWithToken(cmd.Context(), stdout, st, *configFile, strings.TrimSpace(tokenFlag))
			}

			issuer, clientID := inferOIDCConfig(st.cfg.APIURL)
			if oidcIssuer != "" {
				issuer = strings.TrimRight(oidcIssuer, "/")
			}
			if oidcClientID != "" {
				clientID = oidcClientID
			}

			httpClient := &http.Client{Timeout: st.timeout}
			authz, err := startDeviceAuthorization(cmd.Context(), httpClient, issuer, clientID)
			if err != nil {
				return err
			}
			authURL := authz.VerificationURIComplete
			if authURL == "" {
				authURL = authz.VerificationURI
			}
			fmt.Fprintf(stdout, "Open this URL to approve Antfly Cloud CLI login:\n%s\n\nCode: %s\n", authURL, authz.UserCode)
			if noBrowser {
				fmt.Fprintln(stdout)
			} else if err := openBrowser(authURL); err != nil {
				fmt.Fprintf(stdout, "Could not open browser automatically: %v\n", err)
			}
			fmt.Fprintln(stdout, "Waiting for approval...")
			tokens, err := pollDeviceToken(cmd.Context(), httpClient, issuer, clientID, authz)
			if err != nil {
				return err
			}

			client, err := NewClient(st.cfg.APIURL, tokens.IDToken, httpClient)
			if err != nil {
				return err
			}
			user, err := client.CurrentUser(cmd.Context())
			if err != nil {
				return fmt.Errorf("token validation failed: %w", err)
			}
			orgs, err := client.Organizations(cmd.Context())
			if err != nil {
				return fmt.Errorf("token validated, but org lookup failed: %w", err)
			}

			st.cfg.Token = ""
			st.cfg.Auth = AuthConfig{
				Type:         authTypeOIDC,
				Issuer:       issuer,
				ClientID:     clientID,
				IDToken:      tokens.IDToken,
				RefreshToken: tokens.RefreshToken,
				ExpiresAt:    tokenExpiry(tokens.ExpiresIn),
			}
			st.cfg.Org = defaultOrgForLogin(st.cfg.Org, orgs)
			if err := saveConfig(*configFile, st.cfg); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Logged in as %s\n", user.Email)
			if st.cfg.Org != "" {
				fmt.Fprintf(stdout, "Default org: %s\n", st.cfg.Org)
			}
			if len(orgs) > 1 && st.cfg.Org == "" {
				fmt.Fprintln(stdout, "Multiple orgs found; run `antfly-cloud org use <org>`.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tokenFlag, "token", "", "management token to store (avoids browser login)")
	cmd.Flags().StringVar(&oidcIssuer, "oidc-issuer", "", "OIDC issuer URL (default inferred from --api-url)")
	cmd.Flags().StringVar(&oidcClientID, "oidc-client-id", "", "OIDC client ID")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the login URL instead of opening a browser")
	return cmd
}

func loginWithToken(ctx context.Context, stdout io.Writer, st *appState, configFile, token string) error {
	client, err := NewClient(st.cfg.APIURL, token, &http.Client{Timeout: st.timeout})
	if err != nil {
		return err
	}
	orgs, err := client.Organizations(ctx)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}
	var loginMessage string
	if isCloudAFManagementToken(token) {
		loginMessage = "Logged in with Antfly Cloud API key\n"
	} else {
		user, err := client.CurrentUser(ctx)
		if err != nil {
			return fmt.Errorf("token validated, but user lookup failed: %w", err)
		}
		loginMessage = fmt.Sprintf("Logged in as %s\n", user.Email)
	}
	st.cfg.Token = token
	st.cfg.Auth = AuthConfig{}
	st.cfg.Org = defaultOrgForLogin(st.cfg.Org, orgs)
	if err := saveConfig(configFile, st.cfg); err != nil {
		return err
	}
	fmt.Fprint(stdout, loginMessage)
	if st.cfg.Org != "" {
		fmt.Fprintf(stdout, "Default org: %s\n", st.cfg.Org)
	}
	if len(orgs) > 1 && st.cfg.Org == "" {
		fmt.Fprintln(stdout, "Multiple orgs found; run `antfly-cloud org use <org>`.")
	}
	return nil
}
func defaultOrgForLogin(current string, orgs []Organization) string {
	if current == "" {
		if len(orgs) == 1 {
			return orgs[0].ID
		}
		return ""
	}
	for _, org := range orgs {
		if current == org.ID || current == org.Slug {
			return current
		}
	}
	if len(orgs) == 1 {
		return orgs[0].ID
	}
	return ""
}

func newLogoutCommand(stdout io.Writer, configFile *string) *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Remove the stored Antfly Cloud token", RunE: func(cmd *cobra.Command, args []string) error {
		if err := removeConfigToken(*configFile); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Logged out")
		return nil
	}}
}

func newWhoamiCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "whoami", Short: "Show the current authenticated user", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		if isCloudAFManagementToken(st.cfg.bearerToken()) {
			org, err := resolveOrg(cmd.Context(), st)
			if err != nil {
				return err
			}
			result := map[string]any{"principal": "antfly_cloud_api_key", "organization": org, "api": st.cfg.APIURL}
			return st.out.Print(result, func() error {
				fmt.Fprintf(st.out.W, "Principal: Antfly Cloud API key\nOrg: %s (%s)\nAPI: %s\n", org.Name, org.Slug, st.cfg.APIURL)
				return nil
			})
		}
		user, err := st.client.CurrentUser(cmd.Context())
		if err != nil {
			return err
		}
		return st.out.Print(user, func() error {
			fmt.Fprintf(st.out.W, "User: %s\nEmail: %s\nStatus: %s\nAPI: %s\n", user.DisplayName, user.Email, user.Status, st.cfg.APIURL)
			return nil
		})
	}}
}

func newOrgsCommand(stateFor stateFactory, configFile *string) *cobra.Command {
	cmd := &cobra.Command{Use: "orgs", Short: "List organizations", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		orgs, err := st.client.Organizations(cmd.Context())
		if err != nil {
			return err
		}
		return st.out.Print(orgs, func() error {
			rows := make([][]string, 0, len(orgs))
			for _, o := range orgs {
				mark := ""
				if st.cfg.Org == o.ID || st.cfg.Org == o.Slug {
					mark = "*"
				}
				rows = append(rows, []string{mark, o.Name, o.Slug, shortID(o.ID), o.Status})
			}
			table(st.out.W, []string{"", "NAME", "SLUG", "ID", "STATUS"}, rows)
			return nil
		})
	}}
	cmd.AddCommand(&cobra.Command{Use: "use <org-id-or-slug>", Short: "Set the default organization", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		orgs, err := st.client.Organizations(cmd.Context())
		if err != nil {
			return err
		}
		for _, o := range orgs {
			if o.ID == args[0] || o.Slug == args[0] {
				cfg, err := loadConfig(*configFile)
				if err != nil {
					return err
				}
				cfg.Org = o.ID
				if st.cfg.APIURL != "" {
					cfg.APIURL = st.cfg.APIURL
				}
				if err := saveConfig(*configFile, cfg); err != nil {
					return err
				}
				fmt.Fprintf(st.out.W, "Default org set to %s (%s)\n", o.Name, o.ID)
				return nil
			}
		}
		return fmt.Errorf("organization %q not found", args[0])
	}})
	return cmd
}

func newStatusCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show Antfly Cloud account and instance status", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		org, err := resolveOrg(ctx, st)
		if err != nil {
			return err
		}
		instances, err := st.client.Instances(ctx, org.ID)
		if err != nil {
			return err
		}
		usage, err := st.client.Usage(ctx, org.ID)
		if err != nil {
			return err
		}
		result := map[string]any{"organization": org, "instances": instances, "usage": usage}
		var user *User
		if !isCloudAFManagementToken(st.cfg.bearerToken()) {
			user, err = st.client.CurrentUser(ctx)
			if err != nil {
				return err
			}
			result["user"] = user
		}
		return st.out.Print(result, func() error {
			if isCloudAFManagementToken(st.cfg.bearerToken()) {
				fmt.Fprintf(st.out.W, "Principal: Antfly Cloud API key\nOrg:  %s (%s)\n\n", org.Name, org.Slug)
			} else {
				fmt.Fprintf(st.out.W, "User: %s <%s>\nOrg:  %s (%s)\n\n", user.DisplayName, user.Email, org.Name, org.Slug)
			}
			rows := make([][]string, 0, len(instances))
			for _, i := range instances {
				rows = append(rows, []string{i.Name, i.Slug, shortID(i.ID), i.Status, i.Region, i.Mode, displayOrDash(i.VersionPolicy), displayOrDash(i.CurrentAntflyVersion), displayOrDash(i.VersionUpgradeStatus), fmtTime(i.UpdatedAt)})
			}
			if len(rows) == 0 {
				fmt.Fprintln(st.out.W, "No cloud instances.")
			} else {
				table(st.out.W, []string{"NAME", "SLUG", "ID", "STATUS", "REGION", "MODE", "POLICY", "ANTFLY", "UPGRADE", "UPDATED"}, rows)
			}
			fmt.Fprintf(st.out.W, "\nUsage this cycle: queries=%d cpu=%.2fh memory=%.2fGiBh disk=%.2fGiBh storage=%.2fGiBh\n", usage.Totals.Queries, usage.Totals.CPUCoreHours, usage.Totals.MemoryGiBHours, usage.Totals.DiskGiBHours, usage.Totals.StorageGiBHours)
			return nil
		})
	}}
}

func newInstancesCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "instances", Short: "List cloud instances", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		instances, err := st.client.Instances(cmd.Context(), org.ID)
		if err != nil {
			return err
		}
		return st.out.Print(instances, func() error {
			rows := [][]string{}
			for _, i := range instances {
				rows = append(rows, []string{i.Name, i.Slug, shortID(i.ID), i.Status, i.Region, i.Mode, displayOrDash(i.VersionPolicy), displayOrDash(i.CurrentAntflyVersion), displayOrDash(i.VersionUpgradeStatus), fmtTime(i.CreatedAt)})
			}
			table(st.out.W, []string{"NAME", "SLUG", "ID", "STATUS", "REGION", "MODE", "POLICY", "ANTFLY", "UPGRADE", "CREATED"}, rows)
			return nil
		})
	}}
}

func newUsageCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "usage", Short: "Show cloud usage for the selected organization", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		usage, err := st.client.Usage(cmd.Context(), org.ID)
		if err != nil {
			return err
		}
		return st.out.Print(usage, func() error { printUsage(st.out.W, usage); return nil })
	}}
}

func newInstanceCommand(stateFor stateFactory, configFile *string) *cobra.Command {
	cmd := &cobra.Command{Use: "instance", Short: "Inspect a cloud instance"}
	cmd.AddCommand(&cobra.Command{Use: "use <instance-id-or-slug>", Short: "Set the default cloud instance", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		inst, err := resolveInstance(cmd.Context(), st, org.ID, args[0])
		if err != nil {
			return err
		}
		cfg, err := loadConfig(*configFile)
		if err != nil {
			return err
		}
		cfg.Org = org.ID
		cfg.Instance = inst.ID
		if st.cfg.APIURL != "" {
			cfg.APIURL = st.cfg.APIURL
		}
		if err := saveConfig(*configFile, cfg); err != nil {
			return err
		}
		fmt.Fprintf(st.out.W, "Default instance set to %s (%s)\n", inst.Name, inst.ID)
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "get <instance-id-or-slug>", Short: "Show instance details", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		full, err := st.client.Instance(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(full, func() error { printInstance(st.out.W, *full); return nil })
	}})
	cmd.AddCommand(&cobra.Command{Use: "policy <instance-id-or-slug> <pinned|patch_auto|minor_auto|major_manual>", Short: "Set instance Antfly version policy", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		policy := args[1]
		if !validVersionPolicy(policy) {
			return fmt.Errorf("version policy must be one of pinned, patch_auto, minor_auto, or major_manual")
		}
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.UpdateInstanceVersionPolicy(cmd.Context(), org.ID, inst.ID, policy)
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Version policy for %s set to %s\n", updated.Name, displayOrDash(updated.VersionPolicy))
			return nil
		})
	}})
	var targetVersion, targetDigest string
	targetCmd := &cobra.Command{Use: "target <instance-id-or-slug> <image>", Short: "Set a manual Antfly runtime image target", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		image := strings.TrimSpace(args[1])
		if image == "" {
			return fmt.Errorf("target image is required")
		}
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.SetInstanceVersionTarget(cmd.Context(), org.ID, inst.ID, targetVersion, image, targetDigest)
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Antfly target for %s set to %s\n", updated.Name, displayOrDash(updated.TargetAntflyImage))
			if updated.TargetAntflyVersion != "" {
				fmt.Fprintf(st.out.W, "Target version: %s\n", updated.TargetAntflyVersion)
			}
			return nil
		})
	}}
	targetCmd.Flags().StringVar(&targetVersion, "version", "", "Antfly runtime version for the target image")
	targetCmd.Flags().StringVar(&targetDigest, "digest", "", "immutable digest for the target image")
	cmd.AddCommand(targetCmd)
	cmd.AddCommand(&cobra.Command{Use: "clear-target <instance-id-or-slug>", Short: "Clear a pending or failed manual Antfly runtime target", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		updated, err := st.client.ClearInstanceVersionTarget(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(updated, func() error {
			fmt.Fprintf(st.out.W, "Antfly target for %s cleared\n", updated.Name)
			return nil
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "metrics <instance-id-or-slug>", Short: "Show instance metrics", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		m, err := st.client.Metrics(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(m, func() error { printMetrics(st.out.W, *m); return nil })
	}})
	cmd.AddCommand(&cobra.Command{Use: "events <instance-id-or-slug>", Short: "List provisioning events", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		events, err := st.client.Events(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(events, func() error {
			rows := [][]string{}
			for _, e := range events {
				rows = append(rows, []string{fmtTime(e.CreatedAt), e.EventType, e.Message})
			}
			table(st.out.W, []string{"TIME", "TYPE", "MESSAGE"}, rows)
			return nil
		})
	}})
	cmd.AddCommand(&cobra.Command{Use: "connection <instance-id-or-slug>", Aliases: []string{"conn"}, Short: "Show connection URLs", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		conn, err := st.client.Connection(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(conn, func() error {
			fmt.Fprintf(st.out.W, "Status: %s\nProxy URL: %s\nAntfly Inference URL: %s\n", conn.Status, conn.ProxyURL, conn.AntflyInferenceProxyURL)
			return nil
		})
	}})
	return cmd
}

func newConnectionCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "connection <instance-id-or-slug>", Aliases: []string{"conn"}, Short: "Show connection URLs for a cloud instance", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, org, inst, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		conn, err := st.client.Connection(cmd.Context(), org.ID, inst.ID)
		if err != nil {
			return err
		}
		return st.out.Print(conn, func() error {
			fmt.Fprintf(st.out.W, "Status: %s\nProxy URL: %s\nAntfly Inference URL: %s\n", conn.Status, conn.ProxyURL, conn.AntflyInferenceProxyURL)
			return nil
		})
	}}
}

func resolveCommandInstance(ctx context.Context, stateFor stateFactory, ref string) (*appState, Organization, CloudInstance, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	org, err := resolveOrg(ctx, st)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	inst, err := resolveInstance(ctx, st, org.ID, ref)
	if err != nil {
		return nil, Organization{}, CloudInstance{}, err
	}
	return st, org, inst, nil
}

func validVersionPolicy(policy string) bool {
	switch policy {
	case "pinned", "patch_auto", "minor_auto", "major_manual":
		return true
	default:
		return false
	}
}

func printInstance(w io.Writer, i CloudInstance) {
	fmt.Fprintf(w, "Name: %s\nSlug: %s\nID: %s\nStatus: %s\nMode: %s\nRegion: %s\nCreated: %s\nUpdated: %s\nProvisioning started: %s\nProvisioning completed: %s\n", i.Name, i.Slug, i.ID, i.Status, i.Mode, i.Region, fmtTime(i.CreatedAt), fmtTime(i.UpdatedAt), fmtPtrTime(i.ProvisioningStartedAt), fmtPtrTime(i.ProvisioningCompletedAt))
	if i.ProvisioningError != "" {
		fmt.Fprintf(w, "Provisioning error: %s\n", i.ProvisioningError)
	}
	fmt.Fprintf(w, "Version policy: %s\nAntfly version: %s\nAntfly image: %s\nTarget Antfly version: %s\nTarget Antfly image: %s\nVersion upgrade status: %s\nVersion upgrade started: %s\nVersion upgrade completed: %s\n", displayOrDash(i.VersionPolicy), displayOrDash(i.CurrentAntflyVersion), displayOrDash(i.CurrentAntflyImage), displayOrDash(i.TargetAntflyVersion), displayOrDash(i.TargetAntflyImage), displayOrDash(i.VersionUpgradeStatus), fmtPtrTime(i.VersionUpgradeStartedAt), fmtPtrTime(i.VersionUpgradeCompletedAt))
	if i.VersionUpgradeError != "" {
		fmt.Fprintf(w, "Version upgrade error: %s\n", i.VersionUpgradeError)
	}
	fmt.Fprintf(w, "Nodes: metadata=%d data=%d cpu=%s memory=%s metadata_storage=%s data_storage=%s\n", i.NodeConfig.MetadataNodes, i.NodeConfig.DataNodes, i.NodeConfig.CPU, i.NodeConfig.Memory, i.NodeConfig.MetadataStorage, i.NodeConfig.DataStorage)
}

func printMetrics(w io.Writer, m InstanceMetrics) {
	fmt.Fprintf(w, "Status: %s\nStorage used: %s\nDocuments: %d\nTables: %d\nQueries this month: %d\nNodes: %d\n", m.Status, bytesHuman(m.StorageUsedBytes), m.DocumentCount, m.TableCount, m.QueriesThisMonth, m.NodeCount)
}

func printUsage(w io.Writer, u *CloudUsageSummary) {
	fmt.Fprintf(w, "Billing cycle: %s — %s\n", fmtTime(u.BillingCycleStart), fmtTime(u.BillingCycleEnd))
	rows := [][]string{}
	for _, i := range u.Instances {
		rows = append(rows, []string{i.Name, shortID(i.InstanceID), fmt.Sprint(i.Queries), fmt.Sprintf("%.2f", i.CPUCoreHours), fmt.Sprintf("%.2f", i.MemoryGiBHours), fmt.Sprintf("%.2f", i.DiskGiBHours), fmt.Sprintf("%.2f", i.StorageGiBHours)})
	}
	table(w, []string{"INSTANCE", "ID", "QUERIES", "CPU_H", "MEM_GIB_H", "DISK_GIB_H", "STOR_GIB_H"}, rows)
	fmt.Fprintf(w, "\nTotals: queries=%d cpu=%.2fh memory=%.2fGiBh disk=%.2fGiBh storage=%.2fGiBh s3_cold=%.2fGiBh gcs_cold=%.2fGiBh\n", u.Totals.Queries, u.Totals.CPUCoreHours, u.Totals.MemoryGiBHours, u.Totals.DiskGiBHours, u.Totals.StorageGiBHours, u.Totals.S3ColdGiBHours, u.Totals.GCSColdGiBHours)
}
