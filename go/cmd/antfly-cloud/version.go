package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	dirty   = "false"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Dirty   string `json:"dirty"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func currentVersionInfo() versionInfo {
	return versionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		Dirty:   dirty,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
}

func newVersionCommand(out func() Output) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Antfly Cloud CLI version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := currentVersionInfo()
			output := out()
			return output.Print(info, func() error {
				fmt.Fprintf(output.W, "antfly-cloud %s\ncommit: %s\nbuilt: %s\ndirty: %s\ngo: %s\nplatform: %s/%s\n", info.Version, info.Commit, info.Date, info.Dirty, info.Go, info.OS, info.Arch)
				return nil
			})
		},
	}
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long:  "Generate shell completion scripts for bash, zsh, fish, or powershell.",
	}
	cmd.AddCommand(&cobra.Command{Use: "bash", Short: "Generate bash completion", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return root.GenBashCompletion(cmd.OutOrStdout())
	}})
	cmd.AddCommand(&cobra.Command{Use: "zsh", Short: "Generate zsh completion", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return root.GenZshCompletion(cmd.OutOrStdout())
	}})
	cmd.AddCommand(&cobra.Command{Use: "fish", Short: "Generate fish completion", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return root.GenFishCompletion(cmd.OutOrStdout(), true)
	}})
	cmd.AddCommand(&cobra.Command{Use: "powershell", Short: "Generate PowerShell completion", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return root.GenPowerShellCompletion(cmd.OutOrStdout())
	}})
	return cmd
}
