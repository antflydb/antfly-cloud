package sdk

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ProviderAuth never includes secret material. Config contains public connector settings.
type ProviderAuth struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	UserID         *string        `json:"user_id,omitempty"`
	Provider       string         `json:"provider"`
	DisplayName    string         `json:"display_name"`
	AuthType       string         `json:"auth_type"`
	Enabled        bool           `json:"enabled"`
	Status         string         `json:"status"`
	IsDefault      bool           `json:"is_default"`
	HasSecret      bool           `json:"has_secret"`
	Config         map[string]any `json:"config,omitempty"`
	ValidatedAt    *time.Time     `json:"validated_at,omitempty"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type CreateProviderAuthRequest struct {
	Provider    string         `json:"provider"`
	DisplayName string         `json:"display_name"`
	AuthType    string         `json:"auth_type,omitempty"`
	Secret      string         `json:"secret"`
	Config      map[string]any `json:"config"`
	Enabled     *bool          `json:"enabled,omitempty"`
	IsDefault   *bool          `json:"is_default,omitempty"`
}

func credentialAPIPath(org, instance, collection, id string) (string, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return "", err
	}
	path := instanceAPIPath(orgID.String(), instanceID.String()) + "/" + collection
	if id != "" {
		parsed, err := parseUUID(id, "credential")
		if err != nil {
			return "", err
		}
		path += "/" + parsed.String()
	}
	return path, nil
}

func (c *Client) ProviderAuths(ctx context.Context, org, instance string) ([]ProviderAuth, error) {
	path, err := credentialAPIPath(org, instance, "provider-auths", "")
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []ProviderAuth `json:"data"`
	}
	if err := c.requestJSON(ctx, http.MethodGet, path, nil, &result, true); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *Client) CreateProviderAuth(ctx context.Context, org, instance string, req CreateProviderAuthRequest) (*ProviderAuth, error) {
	path, err := credentialAPIPath(org, instance, "provider-auths", "")
	if err != nil {
		return nil, err
	}
	var result ProviderAuth
	if err := c.requestJSON(ctx, http.MethodPost, path, req, &result, true); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteProviderAuth(ctx context.Context, org, instance, id string) error {
	path, err := credentialAPIPath(org, instance, "provider-auths", id)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path, nil, nil, true)
}

// RevokeProviderAuth removes usable credentials while retaining import history.
// Deletion is not a substitute: archived jobs can still reference the credential.
func (c *Client) RevokeProviderAuth(ctx context.Context, org, instance, id string) (*ProviderAuth, error) {
	path, err := credentialAPIPath(org, instance, "provider-auths", id)
	if err != nil {
		return nil, err
	}
	var result ProviderAuth
	if err := c.requestJSON(ctx, http.MethodPatch, path, map[string]bool{
		"enabled": false, "is_default": false, "clear_secret": true,
	}, &result, true); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) RevokeCloudAPIKey(ctx context.Context, org, instance, id string) error {
	path, err := credentialAPIPath(org, instance, "api-keys", id)
	if err != nil {
		return err
	}
	return c.requestJSON(ctx, http.MethodDelete, path, nil, nil, true)
}

type ProviderAuthObject struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag,omitempty"`
	LastModified time.Time `json:"last_modified,omitempty"`
}

type ProviderAuthObjects struct {
	Objects   []ProviderAuthObject `json:"objects"`
	Prefixes  []string             `json:"prefixes"`
	Truncated bool                 `json:"truncated"`
}

func (c *Client) ProviderAuthObjects(ctx context.Context, org, instance, id, bucket, prefix string, limit int, recursive bool) (*ProviderAuthObjects, error) {
	path, err := credentialAPIPath(org, instance, "provider-auths", id)
	if err != nil {
		return nil, err
	}
	query := url.Values{"bucket": {bucket}, "prefix": {prefix}, "limit": {strconv.Itoa(limit)}, "recursive": {strconv.FormatBool(recursive)}}
	var result ProviderAuthObjects
	if err := c.requestJSON(ctx, http.MethodGet, path+"/objects?"+query.Encode(), nil, &result, true); err != nil {
		return nil, err
	}
	return &result, nil
}
