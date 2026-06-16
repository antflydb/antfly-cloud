package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type antflyContext struct {
	URL       string     `json:"url"`
	Token     string     `json:"token"`
	Org       string     `json:"org"`
	Instance  string     `json:"instance"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func newAntflyContextCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "context [instance-id-or-slug]", Short: "Show Antfly CLI connection context", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, ctx, err := antflyContextForCommand(cmd, stateFor, args)
		if err != nil {
			return err
		}
		return st.out.Print(ctx, func() error {
			fmt.Fprintf(st.out.W, "Org: %s\nInstance: %s\nURL: %s\n", ctx.Org, ctx.Instance, ctx.URL)
			if ctx.ExpiresAt != nil {
				fmt.Fprintf(st.out.W, "Token expires: %s\n", fmtTime(*ctx.ExpiresAt))
			}
			fmt.Fprintln(st.out.W, "Use `antfly-cloud context --json` or `antfly-cloud env` to export credentials.")
			return nil
		})
	}}
}

func newAntflyEnvCommand(stateFor stateFactory) *cobra.Command {
	return &cobra.Command{Use: "env [instance-id-or-slug]", Short: "Print shell exports for the Antfly CLI", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		st, ctx, err := antflyContextForCommand(cmd, stateFor, args)
		if err != nil {
			return err
		}
		fmt.Fprintf(st.out.W, "export ANTFLY_URL=%s\n", shellQuote(ctx.URL))
		fmt.Fprintf(st.out.W, "export ANTFLY_TOKEN=%s\n", shellQuote(ctx.Token))
		return nil
	}}
}

func antflyContextForCommand(cmd *cobra.Command, stateFor stateFactory, args []string) (*appState, *antflyContext, error) {
	st, err := stateFor(true)
	if err != nil {
		return nil, nil, err
	}
	ref := ""
	if len(args) > 0 {
		ref = args[0]
	}
	ctx, err := resolveAntflyContext(cmd, st, ref)
	if err != nil {
		return nil, nil, err
	}
	return st, ctx, nil
}

func resolveAntflyContext(cmd *cobra.Command, st *appState, instanceRef string) (*antflyContext, error) {
	token := st.cfg.bearerToken()
	if isCloudAFManagementToken(token) {
		return nil, fmt.Errorf("Antfly Cloud management API keys (`cloudaf_*`) cannot be used as Antfly data-plane tokens; run `antfly-cloud login` with device auth")
	}
	org, err := resolveOrg(cmd.Context(), st)
	if err != nil {
		return nil, err
	}
	inst, err := resolveActiveInstance(cmd.Context(), st, org.ID, instanceRef)
	if err != nil {
		return nil, err
	}
	conn, err := st.client.Connection(cmd.Context(), org.ID, inst.ID)
	if err != nil {
		return nil, err
	}
	proxyURL, err := absoluteCloudURL(st.cfg.APIURL, conn.ProxyURL)
	if err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if !st.cfg.Auth.ExpiresAt.IsZero() {
		expiresAt = &st.cfg.Auth.ExpiresAt
	}
	return &antflyContext{
		URL:       proxyURL,
		Token:     token,
		Org:       org.Slug,
		Instance:  inst.Slug,
		ExpiresAt: expiresAt,
	}, nil
}

func absoluteCloudURL(apiURL, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("connection response did not include a proxy URL")
	}
	if u, err := url.Parse(value); err == nil && u.IsAbs() {
		return strings.TrimRight(value, "/"), nil
	}
	base, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("parse API URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("API URL %q is not absolute", apiURL)
	}
	return base.Scheme + "://" + base.Host + "/" + strings.TrimLeft(value, "/"), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
