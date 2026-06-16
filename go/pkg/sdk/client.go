//go:generate go tool oapi-codegen --config=cfg.yaml ../../../openapi.yaml

package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/antflydb/antfly-cloud/go/pkg/sdk/oapi"
	"github.com/google/uuid"
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

type Organization struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

type CloudInstance struct {
	ID                        string     `json:"id"`
	OrganizationID            string     `json:"organization_id"`
	Name                      string     `json:"name"`
	Slug                      string     `json:"slug"`
	Mode                      string     `json:"mode"`
	Status                    string     `json:"status"`
	Region                    string     `json:"region"`
	NodeConfig                NodeConfig `json:"node_config"`
	VersionPolicy             string     `json:"version_policy"`
	CurrentAntflyVersion      string     `json:"current_antfly_version"`
	CurrentAntflyImage        string     `json:"current_antfly_image"`
	CurrentAntflyImageDigest  string     `json:"current_antfly_image_digest"`
	TargetAntflyVersion       string     `json:"target_antfly_version"`
	TargetAntflyImage         string     `json:"target_antfly_image"`
	TargetAntflyImageDigest   string     `json:"target_antfly_image_digest"`
	VersionUpgradeStatus      string     `json:"version_upgrade_status"`
	VersionUpgradeError       string     `json:"version_upgrade_error"`
	VersionUpgradeStartedAt   *time.Time `json:"version_upgrade_started_at"`
	VersionUpgradeCompletedAt *time.Time `json:"version_upgrade_completed_at"`
	ProvisioningStartedAt     *time.Time `json:"provisioning_started_at"`
	ProvisioningCompletedAt   *time.Time `json:"provisioning_completed_at"`
	ProvisioningError         string     `json:"provisioning_error"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}

type NodeConfig struct {
	MetadataNodes     int    `json:"metadata_nodes"`
	DataNodes         int    `json:"data_nodes"`
	CPU               string `json:"cpu"`
	Memory            string `json:"memory"`
	Storage           string `json:"storage"`
	MetadataStorage   string `json:"metadata_storage"`
	DataStorage       string `json:"data_storage"`
	ReplicationFactor int    `json:"replication_factor"`
}

type InstanceMetrics struct {
	InstanceID       string `json:"instance_id"`
	Status           string `json:"status"`
	StorageUsedBytes int64  `json:"storage_used_bytes"`
	DocumentCount    int64  `json:"document_count"`
	TableCount       int    `json:"table_count"`
	QueriesThisMonth int    `json:"queries_this_month"`
	NodeCount        int    `json:"node_count"`
}

type ProvisioningEvent struct {
	ID              string                 `json:"id"`
	CloudInstanceID string                 `json:"cloud_instance_id"`
	EventType       string                 `json:"event_type"`
	Message         string                 `json:"message"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedAt       time.Time              `json:"created_at"`
}

type ConnectionDetails struct {
	ProxyURL                string `json:"proxy_url"`
	AntflyInferenceProxyURL string `json:"antfly_inference_proxy_url"`
	Status                  string `json:"status"`
}

type CloudUsageSummary struct {
	BillingCycleStart time.Time            `json:"billing_cycle_start"`
	BillingCycleEnd   time.Time            `json:"billing_cycle_end"`
	Instances         []CloudInstanceUsage `json:"instances"`
	Totals            CloudUsageTotals     `json:"totals"`
}

type CloudInstanceUsage struct {
	InstanceID      string  `json:"instance_id"`
	Name            string  `json:"name"`
	Queries         int     `json:"queries"`
	CPUCoreHours    float64 `json:"cpu_core_hours"`
	MemoryGiBHours  float64 `json:"memory_gib_hours"`
	DiskGiBHours    float64 `json:"disk_gib_hours"`
	StorageGiBHours float64 `json:"storage_gib_hours"`
	S3ColdGiBHours  float64 `json:"s3_cold_gib_hours"`
	GCSColdGiBHours float64 `json:"gcs_cold_gib_hours"`
}

type CloudUsageTotals struct {
	Queries         int     `json:"queries"`
	CPUCoreHours    float64 `json:"cpu_core_hours"`
	MemoryGiBHours  float64 `json:"memory_gib_hours"`
	DiskGiBHours    float64 `json:"disk_gib_hours"`
	StorageGiBHours float64 `json:"storage_gib_hours"`
	S3ColdGiBHours  float64 `json:"s3_cold_gib_hours"`
	GCSColdGiBHours float64 `json:"gcs_cold_gib_hours"`
}

// APIError represents a non-2xx Antfly Cloud API response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("api returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("api returned HTTP %d: %s", e.StatusCode, e.Body)
}

// Client is a small ergonomic wrapper around the generated Antfly Cloud OpenAPI client.
type Client struct {
	client *oapi.ClientWithResponses
}

// NewClient creates a Antfly Cloud API client. baseURL should include the /api/v1 prefix.
func NewClient(baseURL, bearerToken string, httpClient *http.Client) (*Client, error) {
	opts := []oapi.ClientOption{}
	if httpClient != nil {
		opts = append(opts, oapi.WithHTTPClient(httpClient))
	}
	if bearerToken != "" {
		opts = append(opts, oapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+bearerToken)
			return nil
		}))
	}
	client, err := oapi.NewClientWithResponses(strings.TrimRight(baseURL, "/"), opts...)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	resp, err := c.client.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapUser(*resp.JSON200), nil
}

func (c *Client) Organizations(ctx context.Context) ([]Organization, error) {
	resp, err := c.client.ListOrganizationsWithResponse(ctx, &oapi.ListOrganizationsParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]Organization, 0, len(resp.JSON200.Data))
	for _, org := range resp.JSON200.Data {
		out = append(out, mapOrganization(org))
	}
	return out, nil
}

func (c *Client) Instance(ctx context.Context, org, instance string) (*CloudInstance, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.GetCloudInstanceWithResponse(ctx, orgID, instanceID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudInstance(*resp.JSON200), nil
}

func (c *Client) UpdateInstanceVersionPolicy(ctx context.Context, org, instance, policy string) (*CloudInstance, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateCloudInstanceWithResponse(ctx, orgID, instanceID, oapi.UpdateCloudInstanceJSONRequestBody{
		VersionPolicy: oapi.CloudInstanceVersionPolicy(policy),
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudInstance(*resp.JSON200), nil
}

func (c *Client) SetInstanceVersionTarget(ctx context.Context, org, instance, version, image, digest string) (*CloudInstance, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateCloudInstanceWithResponse(ctx, orgID, instanceID, oapi.UpdateCloudInstanceJSONRequestBody{
		TargetAntflyVersion:     strings.TrimSpace(version),
		TargetAntflyImage:       strings.TrimSpace(image),
		TargetAntflyImageDigest: strings.TrimSpace(digest),
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudInstance(*resp.JSON200), nil
}

func (c *Client) ClearInstanceVersionTarget(ctx context.Context, org, instance string) (*CloudInstance, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateCloudInstanceWithResponse(ctx, orgID, instanceID, oapi.UpdateCloudInstanceJSONRequestBody{
		ClearAntflyVersionTarget: true,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudInstance(*resp.JSON200), nil
}

func (c *Client) Instances(ctx context.Context, org string) ([]CloudInstance, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ListCloudInstancesWithResponse(ctx, orgID, &oapi.ListCloudInstancesParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]CloudInstance, 0, len(resp.JSON200.Data))
	for _, inst := range resp.JSON200.Data {
		out = append(out, *mapCloudInstance(inst))
	}
	return out, nil
}

func (c *Client) Metrics(ctx context.Context, org, instance string) (*InstanceMetrics, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.GetCloudInstanceMetricsWithResponse(ctx, orgID, instanceID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapMetrics(*resp.JSON200), nil
}

func (c *Client) Events(ctx context.Context, org, instance string) ([]ProvisioningEvent, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ListCloudInstanceEventsWithResponse(ctx, orgID, instanceID, &oapi.ListCloudInstanceEventsParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]ProvisioningEvent, 0, len(resp.JSON200.Data))
	for _, event := range resp.JSON200.Data {
		out = append(out, mapEvent(event))
	}
	return out, nil
}

func (c *Client) Connection(ctx context.Context, org, instance string) (*ConnectionDetails, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.GetCloudInstanceConnectionWithResponse(ctx, orgID, instanceID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return &ConnectionDetails{ProxyURL: resp.JSON200.ProxyUrl, AntflyInferenceProxyURL: resp.JSON200.AntflyInferenceProxyUrl, Status: string(resp.JSON200.Status)}, nil
}

func (c *Client) Usage(ctx context.Context, org string) (*CloudUsageSummary, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.GetCloudUsageWithResponse(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapUsage(*resp.JSON200), nil
}

func apiError(status int, body []byte) error {
	return &APIError{StatusCode: status, Body: strings.TrimSpace(string(body))}
}

func parseOrgInstance(org, instance string) (oapi.OrgId, oapi.InstanceId, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return oapi.OrgId{}, oapi.InstanceId{}, err
	}
	instanceID, err := parseUUID(instance, "instance")
	if err != nil {
		return oapi.OrgId{}, oapi.InstanceId{}, err
	}
	return orgID, instanceID, nil
}

func parseUUID(raw, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid %s id %q: %w", name, raw, err)
	}
	return id, nil
}

func mapUser(u oapi.User) *User {
	return &User{ID: u.Id.String(), Email: string(u.Email), DisplayName: u.DisplayName, Status: string(u.Status)}
}

func mapOrganization(o oapi.Organization) Organization {
	return Organization{ID: o.Id.String(), Name: o.Name, Slug: o.Slug, Status: string(o.Status)}
}

func mapCloudInstance(i oapi.CloudInstance) *CloudInstance {
	return &CloudInstance{
		ID:                        i.Id.String(),
		OrganizationID:            i.OrganizationId.String(),
		Name:                      i.Name,
		Slug:                      i.Slug,
		Mode:                      string(i.Mode),
		Status:                    string(i.Status),
		Region:                    i.Region,
		NodeConfig:                mapNodeConfig(i.NodeConfig),
		VersionPolicy:             string(i.VersionPolicy),
		CurrentAntflyVersion:      i.CurrentAntflyVersion,
		CurrentAntflyImage:        i.CurrentAntflyImage,
		CurrentAntflyImageDigest:  i.CurrentAntflyImageDigest,
		TargetAntflyVersion:       i.TargetAntflyVersion,
		TargetAntflyImage:         i.TargetAntflyImage,
		TargetAntflyImageDigest:   i.TargetAntflyImageDigest,
		VersionUpgradeStatus:      string(i.VersionUpgradeStatus),
		VersionUpgradeError:       i.VersionUpgradeError,
		VersionUpgradeStartedAt:   optionalTime(i.VersionUpgradeStartedAt),
		VersionUpgradeCompletedAt: optionalTime(i.VersionUpgradeCompletedAt),
		ProvisioningStartedAt:     optionalTime(i.ProvisioningStartedAt),
		ProvisioningCompletedAt:   optionalTime(i.ProvisioningCompletedAt),
		ProvisioningError:         i.ProvisioningError,
		CreatedAt:                 i.CreatedAt,
		UpdatedAt:                 i.UpdatedAt,
	}
}

func mapNodeConfig(n oapi.NodeConfig) NodeConfig {
	return NodeConfig{
		MetadataNodes:     n.MetadataNodes,
		DataNodes:         n.DataNodes,
		CPU:               n.Cpu,
		Memory:            n.Memory,
		Storage:           n.Storage,
		MetadataStorage:   n.MetadataStorage,
		DataStorage:       n.DataStorage,
		ReplicationFactor: n.ReplicationFactor,
	}
}

func mapMetrics(m oapi.InstanceMetrics) *InstanceMetrics {
	return &InstanceMetrics{InstanceID: m.InstanceId.String(), Status: string(m.Status), StorageUsedBytes: m.StorageUsedBytes, DocumentCount: m.DocumentCount, TableCount: m.TableCount, QueriesThisMonth: m.QueriesThisMonth, NodeCount: m.NodeCount}
}

func mapEvent(e oapi.ProvisioningEvent) ProvisioningEvent {
	return ProvisioningEvent{ID: e.Id.String(), CloudInstanceID: e.CloudInstanceId.String(), EventType: e.EventType, Message: e.Message, Metadata: e.Metadata, CreatedAt: e.CreatedAt}
}

func mapUsage(u oapi.CloudUsageSummary) *CloudUsageSummary {
	instances := make([]CloudInstanceUsage, 0, len(u.Instances))
	for _, inst := range u.Instances {
		instances = append(instances, CloudInstanceUsage{InstanceID: inst.InstanceId.String(), Name: inst.Name, Queries: inst.Queries, CPUCoreHours: inst.CpuCoreHours, MemoryGiBHours: inst.MemoryGibHours, DiskGiBHours: inst.DiskGibHours, StorageGiBHours: inst.DiskUsageGibHours, S3ColdGiBHours: inst.ObjectstoreGibHours, GCSColdGiBHours: 0})
	}
	return &CloudUsageSummary{BillingCycleStart: u.BillingCycleStart, BillingCycleEnd: u.BillingCycleEnd, Instances: instances, Totals: CloudUsageTotals{Queries: u.Totals.Queries, CPUCoreHours: u.Totals.CpuCoreHours, MemoryGiBHours: u.Totals.MemoryGibHours, DiskGiBHours: u.Totals.DiskGibHours, StorageGiBHours: u.Totals.DiskUsageGibHours, S3ColdGiBHours: u.Totals.ObjectstoreGibHours, GCSColdGiBHours: 0}}
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
