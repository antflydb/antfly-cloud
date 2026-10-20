package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// BetaConnectedSourceCapabilities describes implemented operations, not grants
// or live provider health. This opt-in beta contract may change.
type BetaConnectedSourceCapabilities struct {
	Stage               string   `json:"stage"`
	Operations          []string `json:"operations"`
	ConnectionProviders []string `json:"connection_providers"`
	SourceManagement    bool     `json:"source_management"`
	Search              bool     `json:"search"`
}

// BetaSourceConnection is saved connection metadata. It contains no provider
// configuration or credentials and does not imply that content has been shared.
type BetaSourceConnection struct {
	ID          string     `json:"id"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	AuthType    string     `json:"auth_type"`
	Enabled     bool       `json:"enabled"`
	Status      string     `json:"status"`
	ValidatedAt *time.Time `json:"validated_at,omitempty"`
}

func (c *Client) BetaConnectedSourceCapabilities(ctx context.Context, org, instance string) (*BetaConnectedSourceCapabilities, error) {
	var result struct {
		Data *BetaConnectedSourceCapabilities `json:"data"`
	}
	if err := c.getConnectedSourcesBeta(ctx, org, instance, "capabilities", &result); err != nil {
		return nil, err
	}
	if result.Data == nil || result.Data.Stage != "beta" || result.Data.Operations == nil || result.Data.ConnectionProviders == nil {
		return nil, fmt.Errorf("invalid connected sources beta capabilities response")
	}
	return result.Data, nil
}

func (c *Client) BetaSourceConnections(ctx context.Context, org, instance string) ([]BetaSourceConnection, error) {
	var result struct {
		Data *[]BetaSourceConnection `json:"data"`
	}
	if err := c.getConnectedSourcesBeta(ctx, org, instance, "connections", &result); err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, fmt.Errorf("invalid connected sources beta connections response")
	}
	return *result.Data, nil
}

// Beta routes remain outside the stable generated contract. Reuse its transport
// and request editors so authentication, cancellation and timeouts stay shared.
func (c *Client) getConnectedSourcesBeta(ctx context.Context, org, instance, endpoint string, result any) error {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return err
	}
	base, err := url.Parse(c.transport.Server)
	if err != nil {
		return err
	}
	relative := &url.URL{Path: "organizations/" + orgID.String() + "/cloud/instances/" + instanceID.String() + "/beta/connected-sources/" + endpoint}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.ResolveReference(relative).String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	for _, edit := range c.transport.RequestEditors {
		if err := edit(ctx, request); err != nil {
			return err
		}
	}
	response, err := c.transport.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// Error bodies can come from proxies as well as the API. Do not echo them
		// into terminal output where they could expose credentials/configuration.
		return &APIError{StatusCode: response.StatusCode}
	}
	const maxResponseBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("connected sources beta response exceeds 1 MiB")
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("invalid connected sources beta JSON response")
	}
	return nil
}
