package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/antflydb/antfly/go/pkg/sources/management"
	"github.com/antflydb/antfly/go/pkg/sources/management/commands"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func sourceManifestID(instanceID, key string) (string, error) {
	instance, err := uuid.Parse(instanceID)
	if err != nil {
		return "", &commands.Error{Code: commands.CodeInvalid, Message: fmt.Sprintf("invalid instance UUID %q: %v", instanceID, err)}
	}
	return uuid.NewSHA1(instance, []byte("antfly-cloud/sources/"+key)).String(), nil
}

func newSourceManifestCommand(action string, stateFor stateFactory) *cobra.Command {
	var file string
	short := "Plan source changes from a manifest without writing"
	if action == "apply" {
		short = "Apply source changes from a manifest"
	}
	command := &cobra.Command{
		Use:   action + " --file PATH",
		Short: short,
		Long: short + `.

Read a version 1 YAML or JSON manifest (at most 1 MiB). Use --file - for stdin.
Source keys determine stable IDs within the selected instance; changing a name
updates the same source. Connections accept UUIDs or exact enabled display names.
New sources default to native publication and private access for the current user.
Sources absent from the manifest are left untouched.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if file == "" {
				return &commands.Error{Code: commands.CodeInvalid, Message: "--file is required"}
			}
			manifest, err := readSourceManifest(cmd, file)
			if err != nil {
				return err
			}
			st, org, instance, err := sourceCommandInstance(cmd.Context(), stateFor)
			if err != nil {
				return err
			}
			env, err := sourceManagementForInstance(stateFor, st, org.ID, instance.ID)
			if err != nil {
				return err
			}
			opts := sourceManifestOptions(st, org.ID, instance.ID)
			if action == "plan" {
				_, err = commands.PlanManifest(cmd.Context(), env, manifest, opts)
			} else {
				_, err = commands.ApplyManifest(cmd.Context(), env, manifest, opts)
			}
			return err
		},
	}
	command.Flags().StringVar(&file, "file", "", "manifest path (.json or YAML); - reads stdin (required)")
	return command
}

func sourceManifestOptions(st *appState, orgID, instanceID string) commands.PlanOptions {
	var connections []sdk.SourceConnection
	var loaded bool
	return commands.PlanOptions{
		Identity: func(key string) (string, error) {
			return sourceManifestID(instanceID, key)
		},
		ResolveConnection: func(ctx context.Context, ref string) (string, error) {
			if isUUID(ref) {
				return ref, nil
			}
			if !loaded {
				var err error
				connections, err = listSourceConnectionsByName(ctx, st, orgID, instanceID)
				if err != nil {
					return "", err
				}
				loaded = true
			}
			return matchSourceConnectionName(connections, ref)
		},
		DefaultPermissions: func(ctx context.Context) (management.Permissions, error) {
			user, err := st.client.CurrentUser(ctx)
			if err != nil {
				return management.Permissions{}, err
			}
			return management.Permissions{Public: false, Readers: []string{"user:" + user.ID}}, nil
		},
	}
}

// matchSourceConnectionName returns the ID of the single enabled connection whose
// display name is exactly ref.
func matchSourceConnectionName(connections []sdk.SourceConnection, ref string) (string, error) {
	var matches []string
	for _, connection := range connections {
		if connection.Enabled && connection.DisplayName == ref {
			matches = append(matches, connection.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", &commands.Error{Code: commands.CodeNotFound, Message: fmt.Sprintf("connection %q not found; run antfly-cloud connections list", ref)}
	case 1:
		return matches[0], nil
	default:
		return "", &commands.Error{Code: commands.CodeInvalid, Message: fmt.Sprintf("connection %q is ambiguous: %s", ref, strings.Join(matches, ", "))}
	}
}

func readSourceManifest(cmd *cobra.Command, file string) (commands.Manifest, error) {
	invalid := func(err error) (commands.Manifest, error) {
		return commands.Manifest{}, &commands.Error{Code: commands.CodeInvalid, Message: fmt.Sprintf("manifest %q: %v", file, err)}
	}
	reader := cmd.InOrStdin()
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return invalid(err)
		}
		defer f.Close()
		reader = f
	}
	const maxSize = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return invalid(err)
	}
	if len(raw) > maxSize {
		return invalid(errors.New("manifest exceeds 1 MiB"))
	}
	if file == "-" || filepath.Ext(file) != ".json" {
		if raw, err = sourceManifestYAMLToJSON(raw); err != nil {
			return invalid(err)
		}
	}
	return commands.DecodeManifest(bytes.NewReader(raw))
}

// sourceManifestYAMLToJSON re-encodes a single YAML document as JSON so it can
// go through the same strict manifest decoder as .json files.
func sourceManifestYAMLToJSON(raw []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("expected exactly one YAML document")
		}
		return nil, err
	}
	value, err := sourceManifestJSONValue(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// sourceManifestJSONValue preserves every key while rejecting YAML-only map keys.
func sourceManifestJSONValue(value any) (any, error) {
	switch value := value.(type) {
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("non-string YAML map key %v", key)
			}
			converted, err := sourceManifestJSONValue(child)
			if err != nil {
				return nil, err
			}
			result[name] = converted
		}
		return result, nil
	case map[string]any:
		for key, child := range value {
			converted, err := sourceManifestJSONValue(child)
			if err != nil {
				return nil, err
			}
			value[key] = converted
		}
	case []any:
		for i, child := range value {
			converted, err := sourceManifestJSONValue(child)
			if err != nil {
				return nil, err
			}
			value[i] = converted
		}
	}
	return value, nil
}

// listSourceConnectionsByName lists connections so a display name can be
// matched. Listing needs instance-wide connection read access; callers limited
// to a destination table can still reference a connection by its ID.
func listSourceConnectionsByName(ctx context.Context, st *appState, orgID, instanceID string) ([]sdk.SourceConnection, error) {
	connections, err := st.client.SourceConnections(ctx, orgID, instanceID, "")
	var apiErr *sdk.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("looking up a source connection by name requires permission to list connections; reference it by ID instead: %w", err)
	}
	return connections, err
}
