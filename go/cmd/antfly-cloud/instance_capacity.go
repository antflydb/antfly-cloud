package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk"
	"github.com/antflydb/antfly-cloud/go/pkg/sdk/oapi"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func readInstanceRequest(command *cobra.Command, filename string, target any) error {
	var reader io.Reader = command.InOrStdin()
	if filename != "-" {
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("request exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain exactly one JSON object")
	}
	return nil
}

func newInstanceCreateCommand(stateFor stateFactory) *cobra.Command {
	var filename string
	var estimateOnly bool
	command := &cobra.Command{Use: "create", Short: "Create an admitted managed instance from an API request file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var request sdk.CreateCloudInstanceRequest
		if err := readInstanceRequest(cmd, filename, &request); err != nil {
			return err
		}
		if strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.Slug) == "" {
			return fmt.Errorf("name and slug are required")
		}
		state, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), state)
		if err != nil {
			return err
		}
		estimate, err := state.client.EstimateCloudCredits(cmd.Context(), org.ID, sdk.CloudCreditEstimateRequest{Operation: oapi.CloudCreditEstimateRequestOperationCreateInstance, Mode: request.Mode, Tier: request.Tier, NodeConfig: request.NodeConfig, Region: request.Region})
		if err != nil {
			return err
		}
		if estimateOnly {
			return printCapacityEstimate(state, estimate)
		}
		if !estimate.Allowed {
			return fmt.Errorf("instance creation denied: %s", estimate.Reason)
		}
		instance, err := state.client.CreateInstance(cmd.Context(), org.ID, request)
		if err != nil {
			return err
		}
		return state.out.Print(instance, func() error { printInstance(state.out.W, *instance); return nil })
	}}
	command.Flags().StringVar(&filename, "request", "", "CreateCloudInstanceRequest JSON file, or - for stdin")
	command.Flags().BoolVar(&estimateOnly, "estimate", false, "Print the server's credit/admission estimate without creating an instance")
	_ = command.MarkFlagRequired("request")
	return command
}

type capacityRequest struct {
	Mode       sdk.CloudInstanceMode `json:"mode,omitempty"`
	Tier       sdk.CloudInstanceTier `json:"tier,omitempty"`
	NodeConfig sdk.NodeConfigUpdate  `json:"node_config,omitempty"`
}

func newInstanceResizeCommand(stateFor stateFactory) *cobra.Command {
	var filename string
	var estimateOnly bool
	command := &cobra.Command{Use: "resize <instance-id-or-slug>", Short: "Estimate or apply managed capacity changes", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var request capacityRequest
		if err := readInstanceRequest(cmd, filename, &request); err != nil {
			return err
		}
		if request.Mode == "" && request.Tier == "" && reflect.DeepEqual(request.NodeConfig, sdk.NodeConfigUpdate{}) {
			return fmt.Errorf("request must contain a capacity change")
		}
		state, org, instance, err := resolveCommandInstance(cmd.Context(), stateFor, args[0])
		if err != nil {
			return err
		}
		id, err := uuid.Parse(instance.ID)
		if err != nil {
			return err
		}
		estimate, err := state.client.EstimateCloudCredits(cmd.Context(), org.ID, sdk.CloudCreditEstimateRequest{Operation: oapi.CloudCreditEstimateRequestOperationUpdateInstance, InstanceId: id, Mode: request.Mode, Tier: request.Tier, NodeConfig: request.NodeConfig})
		if err != nil {
			return err
		}
		if estimateOnly {
			return printCapacityEstimate(state, estimate)
		}
		if !estimate.Allowed {
			return fmt.Errorf("capacity change denied: %s", estimate.Reason)
		}
		updated, err := state.client.UpdateInstance(cmd.Context(), org.ID, instance.ID, sdk.UpdateCloudInstanceRequest{Mode: request.Mode, Tier: request.Tier, NodeConfig: request.NodeConfig})
		if err != nil {
			return err
		}
		return state.out.Print(updated, func() error { printInstance(state.out.W, *updated); return nil })
	}}
	command.Flags().StringVar(&filename, "request", "", "Capacity update JSON file, or - for stdin")
	command.Flags().BoolVar(&estimateOnly, "estimate", false, "Print the server's credit/admission estimate without resizing")
	_ = command.MarkFlagRequired("request")
	return command
}

func printCapacityEstimate(state *appState, estimate *sdk.CloudCreditEstimate) error {
	return state.out.Print(estimate, func() error {
		fmt.Fprintf(state.out.W, "Allowed: %t\nMonthly commitment: %d cents\nReason: %s\n", estimate.Allowed, estimate.MonthlyCommitmentCents, estimate.Reason)
		return nil
	})
}
