package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

type appState struct {
	cfg     Config
	client  *Client
	out     Output
	timeout time.Duration
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	var apiURL, org, instance, configFile string
	var jsonOut bool
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "antfly-cloud",
		Short: "Command-line tools for Antfly Cloud",
		Long: `antfly-cloud is a command-line and TUI client for Antfly Cloud.

Use it to inspect organizations, hosted Antfly instances, usage, metrics,
provisioning events, connection URLs, and managed runtime policy. The existing
Colony backend server command remains separate and is built as colony cloudaf;
this binary is the customer-facing Antfly Cloud CLI.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&configFile, "config", "", "config file or directory (default is $ANTFLY_CLOUD_CONFIG, ./.antfly/cloud/config.yaml, or ~/.antfly/cloud/config.yaml)")
	cmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Antfly Cloud management API URL")
	cmd.PersistentFlags().StringVar(&org, "org", "", "organization ID or slug")
	cmd.PersistentFlags().StringVar(&instance, "instance", "", "cloud instance ID or slug")
	cmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "write JSON output")
	cmd.PersistentFlags().DurationVar(&timeout, "timeout", 15*time.Second, "API request timeout")

	stateFor := func(requireToken bool) (*appState, error) {
		cfg, err := loadConfig(configFile)
		if err != nil {
			return nil, err
		}
		cfg = applyEnv(cfg)
		if apiURL != "" {
			cfg.APIURL = apiURL
		}
		if org != "" {
			cfg.Org = org
		}
		if instance != "" {
			cfg.Instance = instance
		}
		httpClient := &http.Client{Timeout: timeout}
		if requireToken {
			var refreshed bool
			cfg, refreshed, err = refreshConfigIfNeeded(context.Background(), cfg, httpClient)
			if err != nil {
				return nil, err
			}
			if refreshed {
				if err := saveConfig(configFile, cfg); err != nil {
					return nil, err
				}
			}
		}
		token := cfg.bearerToken()
		if requireToken && token == "" {
			return nil, fmt.Errorf("not logged in: run `antfly-cloud login` or set ANTFLY_CLOUD_TOKEN")
		}
		client, err := NewClient(cfg.APIURL, token, httpClient)
		if err != nil {
			return nil, err
		}
		return &appState{cfg: cfg, client: client, out: Output{JSON: jsonOut, W: stdout}, timeout: timeout}, nil
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.AddCommand(newVersionCommand(func() Output { return Output{JSON: jsonOut, W: stdout} }))
	cmd.AddCommand(newCompletionCommand(cmd))
	cmd.AddCommand(newLoginCommand(stdout, stateFor, &configFile))
	cmd.AddCommand(newLogoutCommand(stdout, &configFile))
	cmd.AddCommand(newWhoamiCommand(stateFor))
	cmd.AddCommand(newOrgsCommand(stateFor, &configFile))
	cmd.AddCommand(newStatusCommand(stateFor))
	cmd.AddCommand(newInstancesCommand(stateFor))
	cmd.AddCommand(newInstanceCommand(stateFor, &configFile))
	cmd.AddCommand(newAntflyEnvCommand(stateFor))
	cmd.AddCommand(newAntflyContextCommand(stateFor))
	cmd.AddCommand(newAccessCommand(stateFor))
	cmd.AddCommand(newConnectionCommand(stateFor))
	cmd.AddCommand(newUsageCommand(stateFor))
	cmd.AddCommand(newTUICommand(stateFor))
	return cmd
}

func execute() int {
	cmd := newRootCommand(os.Stdout, os.Stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func resolveOrg(ctx context.Context, st *appState) (Organization, error) {
	orgs, err := st.client.Organizations(ctx)
	if err != nil {
		return Organization{}, err
	}
	if st.cfg.Org != "" {
		for _, o := range orgs {
			if o.ID == st.cfg.Org || o.Slug == st.cfg.Org {
				return o, nil
			}
		}
		return Organization{}, fmt.Errorf("organization %q not found; run `antfly-cloud orgs`", st.cfg.Org)
	}
	if len(orgs) == 1 {
		return orgs[0], nil
	}
	if len(orgs) == 0 {
		return Organization{}, errors.New("no organizations found for this token")
	}
	return Organization{}, errors.New("multiple organizations found; run `antfly-cloud org use <org-id-or-slug>` or pass --org")
}

func resolveInstance(ctx context.Context, st *appState, orgID, ref string) (CloudInstance, error) {
	instances, err := st.client.Instances(ctx, orgID)
	if err != nil {
		return CloudInstance{}, err
	}
	for _, inst := range instances {
		if inst.ID == ref || inst.Slug == ref {
			return inst, nil
		}
	}
	return CloudInstance{}, fmt.Errorf("instance %q not found in org %s", ref, orgID)
}

func resolveActiveInstance(ctx context.Context, st *appState, orgID, ref string) (CloudInstance, error) {
	if ref == "" {
		ref = st.cfg.Instance
	}
	instances, err := st.client.Instances(ctx, orgID)
	if err != nil {
		return CloudInstance{}, err
	}
	if ref != "" {
		for _, inst := range instances {
			if inst.ID == ref || inst.Slug == ref {
				return inst, nil
			}
		}
		return CloudInstance{}, fmt.Errorf("instance %q not found in org %s", ref, orgID)
	}
	if len(instances) == 1 {
		return instances[0], nil
	}
	if len(instances) == 0 {
		return CloudInstance{}, errors.New("no cloud instances found")
	}
	return CloudInstance{}, errors.New("multiple cloud instances found; run `antfly-cloud instance use <instance-id-or-slug>` or pass --instance")
}
