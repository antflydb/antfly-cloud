package sdk

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type SourceConnection struct {
	ID          string         `json:"id"`
	Provider    string         `json:"provider"`
	DisplayName string         `json:"display_name"`
	Status      string         `json:"status"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
}
type SourceInstallation struct {
	AuthorizationURL string `json:"authorization_url"`
	FlowID           string `json:"flow_id"`
}
type SourceInstallationStatus struct {
	Status         string `json:"status"`
	ProviderAuthID string `json:"provider_auth_id"`
}
type SourceResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Readable bool   `json:"readable"`
	Folder   bool   `json:"folder,omitempty"`
}
type SourceResourcePage struct {
	Data       []SourceResource `json:"data"`
	NextCursor string           `json:"next_cursor,omitempty"`
}
type SourcePreview struct {
	Data []struct {
		Path string `json:"path"`
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"data"`
}
type SourceJob struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Mode         string         `json:"mode"`
	Status       string         `json:"status"`
	SourceConfig map[string]any `json:"source_config"`
}
type CreateSourceRequest struct {
	Name            string         `json:"name"`
	ProviderAuthID  string         `json:"provider_auth_id"`
	TargetTable     string         `json:"target_table"`
	Mode            string         `json:"mode"`
	SourceConfig    map[string]any `json:"source_config"`
	RunPolicy       string         `json:"run_policy"`
	ScheduleSeconds int            `json:"schedule_interval_seconds"`
}

func sourceInstancePath(org, instance string) (string, error) {
	for _, id := range []string{org, instance} {
		if _, err := uuid.Parse(id); err != nil {
			return "", fmt.Errorf("source operations require organization and instance UUIDs")
		}
	}
	return "/organizations/" + url.PathEscape(org) + "/cloud/instances/" + url.PathEscape(instance), nil
}
func sourceID(id string) error {
	_, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("source connection ID must be a UUID")
	}
	return nil
}
func (c *Client) sourceDo(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.sourceBase+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.sourceToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *c.sourceHTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return fmt.Errorf("source response exceeded budget")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, data)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
func (c *Client) SourceConnections(ctx context.Context, org, instance, provider string) ([]SourceConnection, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []SourceConnection `json:"data"`
	}
	err = c.sourceDo(ctx, "GET", base+"/provider-auths?provider="+url.QueryEscape(provider), nil, &result)
	return result.Data, err
}
func (c *Client) StartSourceInstallation(ctx context.Context, org, instance, provider, reconnect string, services []string) (SourceInstallation, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return SourceInstallation{}, err
	}
	switch provider {
	case "google_workspace", "slack", "discord":
	default:
		return SourceInstallation{}, fmt.Errorf("unsupported source provider")
	}
	if reconnect != "" {
		if err := sourceID(reconnect); err != nil {
			return SourceInstallation{}, err
		}
	}
	var result SourceInstallation
	err = c.sourceDo(ctx, "POST", base+"/provider-auths/"+provider+"/install", map[string]any{"services": services, "auth_id": reconnect}, &result)
	return result, err
}
func (c *Client) SourceInstallationStatus(ctx context.Context, org, instance, provider, flow string) (SourceInstallationStatus, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return SourceInstallationStatus{}, err
	}
	if len(flow) != 64 || strings.ContainsAny(flow, "/\\?#") {
		return SourceInstallationStatus{}, fmt.Errorf("invalid installation flow")
	}
	var result SourceInstallationStatus
	err = c.sourceDo(ctx, "GET", base+"/provider-auths/"+url.PathEscape(provider)+"/install/"+flow, nil, &result)
	return result, err
}
func (c *Client) SourceResources(ctx context.Context, org, instance, id, resource, parent, cursor string) (SourceResourcePage, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return SourceResourcePage{}, err
	}
	if err := sourceID(id); err != nil {
		return SourceResourcePage{}, err
	}
	q := url.Values{"resource": {resource}, "parent_id": {parent}, "cursor": {cursor}}
	var result SourceResourcePage
	err = c.sourceDo(ctx, "GET", base+"/provider-auths/"+id+"/resources?"+q.Encode(), nil, &result)
	return result, err
}
func (c *Client) PreviewSource(ctx context.Context, org, instance, id string, config map[string]any) (SourcePreview, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return SourcePreview{}, err
	}
	if err := sourceID(id); err != nil {
		return SourcePreview{}, err
	}
	var result SourcePreview
	err = c.sourceDo(ctx, "POST", base+"/provider-auths/"+id+"/preview-source", config, &result)
	return result, err
}
func (c *Client) CreateSource(ctx context.Context, org, instance string, input CreateSourceRequest) (SourceJob, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return SourceJob{}, err
	}
	input.Mode = "source"
	var result SourceJob
	err = c.sourceDo(ctx, "POST", base+"/import-jobs", input, &result)
	return result, err
}
func (c *Client) SourceJobs(ctx context.Context, org, instance string) ([]SourceJob, error) {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []SourceJob `json:"data"`
	}
	if err := c.sourceDo(ctx, "GET", base+"/import-jobs", nil, &result); err != nil {
		return nil, err
	}
	jobs := []SourceJob{}
	for _, job := range result.Data {
		if job.Mode == "source" {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}
func (c *Client) SyncSource(ctx context.Context, org, instance, id string) error {
	base, err := sourceInstancePath(org, instance)
	if err != nil {
		return err
	}
	if err := sourceID(id); err != nil {
		return err
	}
	return c.sourceDo(ctx, "POST", base+"/import-jobs/"+id+"/run", nil, nil)
}

// ValidateSourceInstallationURL allows only the Antfly management API bootstrap.
// Provider consent URLs are issued after browser sign-in, never opened directly by the CLI.
func (c *Client) ValidateSourceInstallationURL(raw, provider, flow string) error {
	if _, err := hex.DecodeString(flow); err != nil || len(flow) != 64 {
		return fmt.Errorf("invalid installation flow")
	}
	if provider != "google_workspace" && provider != "slack" && provider != "discord" {
		return fmt.Errorf("invalid installation provider")
	}
	target, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid installation URL")
	}
	base, err := url.Parse(c.sourceBase)
	if err != nil {
		return err
	}
	local := base.Scheme == "http" && (base.Hostname() == "localhost" || base.Hostname() == "127.0.0.1" || base.Hostname() == "::1")
	expected := strings.TrimRight(base.Path, "/") + "/source-oauth/" + url.PathEscape(provider) + "/install/" + url.PathEscape(flow)
	if (target.Scheme != "https" && !local) || target.Scheme != base.Scheme || target.Host != base.Host || target.User != nil || target.Path != expected || target.RawQuery != "" || target.Fragment != "" {
		return fmt.Errorf("unexpected installation URL; expected Antfly management origin")
	}
	return nil
}
