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
	openapi_types "github.com/oapi-codegen/runtime/types"
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

type OrganizationMember struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	UserID         string                 `json:"user_id"`
	Email          string                 `json:"email,omitempty"`
	DisplayName    string                 `json:"display_name,omitempty"`
	Role           string                 `json:"role"`
	Status         string                 `json:"status"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	InvitedAt      time.Time              `json:"invited_at"`
	JoinedAt       *time.Time             `json:"joined_at,omitempty"`
}

type Invitation struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	Token     string    `json:"token,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type MemberRoleUpdate struct {
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           string    `json:"role"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CloudGroup struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name"`
	Slug           string                 `json:"slug"`
	ExternalID     string                 `json:"external_id,omitempty"`
	Description    string                 `json:"description,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedBy      string                 `json:"created_by,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type CreateCloudGroupRequest struct {
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug,omitempty"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type UpdateCloudGroupRequest struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type CloudGroupMember struct {
	GroupID string    `json:"group_id"`
	UserID  string    `json:"user_id"`
	AddedBy string    `json:"added_by,omitempty"`
	AddedAt time.Time `json:"added_at"`
}

type CloudGrant struct {
	ID                string                 `json:"id"`
	OrganizationID    string                 `json:"organization_id"`
	InstanceID        string                 `json:"cloud_instance_id"`
	SubjectType       string                 `json:"subject_type"`
	SubjectID         string                 `json:"subject_id"`
	TableName         string                 `json:"table_name"`
	Actions           []string               `json:"actions"`
	RowFilter         map[string]interface{} `json:"row_filter,omitempty"`
	RowFilterTemplate map[string]interface{} `json:"row_filter_template,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedBy         string                 `json:"created_by,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type UpsertCloudGrantRequest struct {
	SubjectType       string                 `json:"subject_type"`
	SubjectID         string                 `json:"subject_id"`
	TableName         string                 `json:"table_name,omitempty"`
	Actions           []string               `json:"actions"`
	RowFilter         map[string]interface{} `json:"row_filter,omitempty"`
	RowFilterTemplate map[string]interface{} `json:"row_filter_template,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

type CloudUserAttributes struct {
	OrganizationID      string                 `json:"organization_id"`
	UserID              string                 `json:"user_id"`
	SyncedAttributes    map[string]interface{} `json:"synced_attributes"`
	ManualAttributes    map[string]interface{} `json:"manual_attributes"`
	EffectiveAttributes map[string]interface{} `json:"effective_attributes"`
	Source              string                 `json:"source,omitempty"`
	UpdatedAt           *time.Time             `json:"updated_at,omitempty"`
}

type CloudSCIMGroupSyncRequest = oapi.CloudSCIMGroupSyncRequest
type CloudSCIMGroupInput = oapi.CloudSCIMGroupInput
type CloudSCIMGroupMemberInput = oapi.CloudSCIMGroupMemberInput

type CloudSCIMGroupSyncResult struct {
	GroupsSynced      int `json:"groups_synced"`
	MembershipsSynced int `json:"memberships_synced"`
	AttributesSynced  int `json:"attributes_synced"`
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

func (c *Client) OrganizationMembers(ctx context.Context, org string) ([]OrganizationMember, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	params := &oapi.ListOrganizationMembersParams{Limit: 100}
	resp, err := c.client.ListOrganizationMembersWithResponse(ctx, orgID, params)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]OrganizationMember, 0, len(resp.JSON200.Data))
	for _, member := range resp.JSON200.Data {
		out = append(out, mapOrganizationMember(member))
	}
	return out, nil
}

func (c *Client) InviteOrganizationMember(ctx context.Context, org, email, role string) (*Invitation, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.InviteOrganizationMemberWithResponse(ctx, orgID, oapi.InviteOrganizationMemberJSONRequestBody{
		Email: openapi_types.Email(email),
		Role:  oapi.OrganizationRole(role),
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapInvitation(*resp.JSON201), nil
}

func (c *Client) UpdateMemberRole(ctx context.Context, org, member, role string) (*MemberRoleUpdate, error) {
	orgID, memberID, err := parseOrgMember(org, member)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateMemberRoleWithResponse(ctx, orgID, memberID, oapi.UpdateMemberRoleJSONRequestBody{
		Role: oapi.UpdateMemberRoleJSONBodyRole(role),
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return &MemberRoleUpdate{
		OrganizationID: resp.JSON200.OrganizationId.String(),
		UserID:         resp.JSON200.UserId.String(),
		Role:           resp.JSON200.Role,
		UpdatedAt:      resp.JSON200.UpdatedAt,
	}, nil
}

func (c *Client) UpdateMemberMetadata(ctx context.Context, org, member string, metadata map[string]interface{}) (*OrganizationMember, error) {
	orgID, memberID, err := parseOrgMember(org, member)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateMemberMetadataWithResponse(ctx, orgID, memberID, oapi.UpdateMemberMetadataJSONRequestBody{Metadata: metadata})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	memberOut := mapOrganizationMember(*resp.JSON200)
	return &memberOut, nil
}

func (c *Client) RemoveOrganizationMember(ctx context.Context, org, member string) error {
	orgID, memberID, err := parseOrgMember(org, member)
	if err != nil {
		return err
	}
	resp, err := c.client.RemoveOrganizationMemberWithResponse(ctx, orgID, memberID)
	if err != nil {
		return err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *Client) CloudGroups(ctx context.Context, org string) ([]CloudGroup, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ListCloudGroupsWithResponse(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]CloudGroup, 0, len(resp.JSON200.Data))
	for _, group := range resp.JSON200.Data {
		out = append(out, mapCloudGroup(group))
	}
	return out, nil
}

func (c *Client) CreateCloudGroup(ctx context.Context, org string, req CreateCloudGroupRequest) (*CloudGroup, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.CreateCloudGroupWithResponse(ctx, orgID, oapi.CreateCloudGroupJSONRequestBody{
		Name: req.Name, Slug: req.Slug, Description: req.Description, Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	group := mapCloudGroup(*resp.JSON201)
	return &group, nil
}

func (c *Client) UpdateCloudGroup(ctx context.Context, org, group string, req UpdateCloudGroupRequest) (*CloudGroup, error) {
	orgID, groupID, err := parseOrgGroup(org, group)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateCloudGroupWithResponse(ctx, orgID, groupID, oapi.UpdateCloudGroupJSONRequestBody{
		Name: req.Name, Description: req.Description, Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	groupOut := mapCloudGroup(*resp.JSON200)
	return &groupOut, nil
}

func (c *Client) CloudGroupMembers(ctx context.Context, org, group string) ([]CloudGroupMember, error) {
	orgID, groupID, err := parseOrgGroup(org, group)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ListCloudGroupMembersWithResponse(ctx, orgID, groupID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]CloudGroupMember, 0, len(resp.JSON200.Data))
	for _, member := range resp.JSON200.Data {
		out = append(out, mapCloudGroupMember(member))
	}
	return out, nil
}

func (c *Client) AddCloudGroupMember(ctx context.Context, org, group, user string) (*CloudGroupMember, error) {
	orgID, groupID, err := parseOrgGroup(org, group)
	if err != nil {
		return nil, err
	}
	userID, err := parseUUID(user, "user")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.AddCloudGroupMemberWithResponse(ctx, orgID, groupID, oapi.AddCloudGroupMemberJSONRequestBody{UserId: userID})
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	member := mapCloudGroupMember(*resp.JSON201)
	return &member, nil
}

func (c *Client) RemoveCloudGroupMember(ctx context.Context, org, group, user string) error {
	orgID, groupID, err := parseOrgGroup(org, group)
	if err != nil {
		return err
	}
	userID, err := parseUUID(user, "user")
	if err != nil {
		return err
	}
	resp, err := c.client.RemoveCloudGroupMemberWithResponse(ctx, orgID, groupID, userID)
	if err != nil {
		return err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *Client) CloudGrants(ctx context.Context, org, instance string) ([]CloudGrant, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.ListCloudGrantsWithResponse(ctx, orgID, instanceID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := make([]CloudGrant, 0, len(resp.JSON200.Data))
	for _, grant := range resp.JSON200.Data {
		out = append(out, mapCloudGrant(grant))
	}
	return out, nil
}

func (c *Client) UpsertCloudGrant(ctx context.Context, org, instance string, req UpsertCloudGrantRequest) (*CloudGrant, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	subjectID, err := parseUUID(req.SubjectID, "subject")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpsertCloudGrantWithResponse(ctx, orgID, instanceID, oapi.UpsertCloudGrantJSONRequestBody{
		SubjectType:       oapi.UpsertCloudGrantRequestSubjectType(req.SubjectType),
		SubjectId:         subjectID,
		TableName:         req.TableName,
		Actions:           req.Actions,
		RowFilter:         req.RowFilter,
		RowFilterTemplate: req.RowFilterTemplate,
		Metadata:          req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	grant := mapCloudGrant(*resp.JSON200)
	return &grant, nil
}

func (c *Client) DeleteCloudGrant(ctx context.Context, org, instance, grant string) error {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return err
	}
	grantID, err := parseUUID(grant, "grant")
	if err != nil {
		return err
	}
	resp, err := c.client.DeleteCloudGrantWithResponse(ctx, orgID, instanceID, grantID)
	if err != nil {
		return err
	}
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

func (c *Client) CloudUserAttributes(ctx context.Context, org, user string) (*CloudUserAttributes, error) {
	orgID, userID, err := parseOrgUser(org, user)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.GetCloudUserAttributesWithResponse(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudUserAttributes(*resp.JSON200), nil
}

func (c *Client) UpdateCloudUserAttributes(ctx context.Context, org, user string, manual map[string]interface{}) (*CloudUserAttributes, error) {
	orgID, userID, err := parseOrgUser(org, user)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.UpdateCloudUserAttributesWithResponse(ctx, orgID, userID, oapi.UpdateCloudUserAttributesJSONRequestBody{ManualAttributes: manual})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return mapCloudUserAttributes(*resp.JSON200), nil
}

func (c *Client) SyncCloudSCIMGroups(ctx context.Context, org string, req CloudSCIMGroupSyncRequest) (*CloudSCIMGroupSyncResult, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	resp, err := c.client.SyncCloudSCIMGroupsWithResponse(ctx, orgID, req)
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return &CloudSCIMGroupSyncResult{
		GroupsSynced:      resp.JSON200.GroupsSynced,
		MembershipsSynced: resp.JSON200.MembershipsSynced,
		AttributesSynced:  resp.JSON200.AttributesSynced,
	}, nil
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

func parseOrgMember(org, member string) (oapi.OrgId, oapi.MemberId, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return oapi.OrgId{}, oapi.MemberId{}, err
	}
	memberID, err := parseUUID(member, "member")
	if err != nil {
		return oapi.OrgId{}, oapi.MemberId{}, err
	}
	return orgID, memberID, nil
}

func parseOrgGroup(org, group string) (oapi.OrgId, oapi.GroupId, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return oapi.OrgId{}, oapi.GroupId{}, err
	}
	groupID, err := parseUUID(group, "group")
	if err != nil {
		return oapi.OrgId{}, oapi.GroupId{}, err
	}
	return orgID, groupID, nil
}

func parseOrgUser(org, user string) (oapi.OrgId, oapi.UUID, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return oapi.OrgId{}, oapi.UUID{}, err
	}
	userID, err := parseUUID(user, "user")
	if err != nil {
		return oapi.OrgId{}, oapi.UUID{}, err
	}
	return orgID, userID, nil
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

func mapOrganizationMember(m oapi.OrganizationMember) OrganizationMember {
	return OrganizationMember{
		ID:             m.Id.String(),
		OrganizationID: m.OrganizationId.String(),
		UserID:         m.UserId.String(),
		Email:          string(m.User.Email),
		DisplayName:    m.User.DisplayName,
		Role:           string(m.Role),
		Status:         string(m.Status),
		Metadata:       m.Metadata,
		InvitedAt:      m.InvitedAt,
		JoinedAt:       optionalTime(m.JoinedAt),
	}
}

func mapInvitation(inv oapi.InvitationResponse2) *Invitation {
	return &Invitation{
		ID:        inv.InvitationId.String(),
		Email:     string(inv.Email),
		Role:      string(inv.Role),
		Status:    string(inv.Status),
		Token:     inv.Token,
		ExpiresAt: inv.ExpiresAt,
	}
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

func mapCloudGroup(g oapi.CloudGroup) CloudGroup {
	return CloudGroup{
		ID:             g.Id.String(),
		OrganizationID: g.OrganizationId.String(),
		Name:           g.Name,
		Slug:           g.Slug,
		ExternalID:     g.ExternalId,
		Description:    g.Description,
		Metadata:       g.Metadata,
		CreatedBy:      uuidString(g.CreatedBy),
		CreatedAt:      g.CreatedAt,
		UpdatedAt:      g.UpdatedAt,
	}
}

func mapCloudGroupMember(m oapi.CloudGroupMember) CloudGroupMember {
	return CloudGroupMember{
		GroupID: m.GroupId.String(),
		UserID:  m.UserId.String(),
		AddedBy: uuidString(m.AddedBy),
		AddedAt: m.AddedAt,
	}
}

func mapCloudGrant(g oapi.CloudGrant) CloudGrant {
	return CloudGrant{
		ID:                g.Id.String(),
		OrganizationID:    g.OrganizationId.String(),
		InstanceID:        g.CloudInstanceId.String(),
		SubjectType:       string(g.SubjectType),
		SubjectID:         g.SubjectId.String(),
		TableName:         g.TableName,
		Actions:           g.Actions,
		RowFilter:         g.RowFilter,
		RowFilterTemplate: g.RowFilterTemplate,
		Metadata:          g.Metadata,
		CreatedBy:         uuidString(g.CreatedBy),
		CreatedAt:         g.CreatedAt,
		UpdatedAt:         g.UpdatedAt,
	}
}

func mapCloudUserAttributes(attrs oapi.CloudUserAttributes) *CloudUserAttributes {
	return &CloudUserAttributes{
		OrganizationID:      attrs.OrganizationId.String(),
		UserID:              attrs.UserId.String(),
		SyncedAttributes:    attrs.SyncedAttributes,
		ManualAttributes:    attrs.ManualAttributes,
		EffectiveAttributes: attrs.EffectiveAttributes,
		Source:              attrs.Source,
		UpdatedAt:           optionalTime(attrs.UpdatedAt),
	}
}

func uuidString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
