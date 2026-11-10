package main

import (
	"errors"
	"io"

	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
	"github.com/spf13/cobra"
)

func sourceManagementEndpoint(apiURL, orgID, instanceID string) string {
	return ""
}

func sourceManagement(cmd *cobra.Command, stateFor stateFactory) (commands.Env, *appState, error) {
	return commands.Env{}, nil, errors.New("not implemented")
}

func errorExitCode(err error) int {
	return 0
}

func writeError(w io.Writer, jsonOut bool, err error) {
}
