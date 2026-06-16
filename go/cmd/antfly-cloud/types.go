package main

import antflycloudclient "github.com/antflydb/antfly-cloud/go/pkg/sdk"

type Client = antflycloudclient.Client
type User = antflycloudclient.User
type Organization = antflycloudclient.Organization
type CloudInstance = antflycloudclient.CloudInstance
type NodeConfig = antflycloudclient.NodeConfig
type InstanceMetrics = antflycloudclient.InstanceMetrics
type ProvisioningEvent = antflycloudclient.ProvisioningEvent
type ConnectionDetails = antflycloudclient.ConnectionDetails
type CloudUsageSummary = antflycloudclient.CloudUsageSummary
type CloudInstanceUsage = antflycloudclient.CloudInstanceUsage
type CloudUsageTotals = antflycloudclient.CloudUsageTotals
type OrganizationMember = antflycloudclient.OrganizationMember
type Invitation = antflycloudclient.Invitation
type MemberRoleUpdate = antflycloudclient.MemberRoleUpdate
type CloudGroup = antflycloudclient.CloudGroup
type CreateCloudGroupRequest = antflycloudclient.CreateCloudGroupRequest
type UpdateCloudGroupRequest = antflycloudclient.UpdateCloudGroupRequest
type CloudGroupMember = antflycloudclient.CloudGroupMember
type CloudGrant = antflycloudclient.CloudGrant
type UpsertCloudGrantRequest = antflycloudclient.UpsertCloudGrantRequest
type CloudUserAttributes = antflycloudclient.CloudUserAttributes
type CloudSCIMGroupSyncRequest = antflycloudclient.CloudSCIMGroupSyncRequest
type CloudSCIMGroupInput = antflycloudclient.CloudSCIMGroupInput
type CloudSCIMGroupMemberInput = antflycloudclient.CloudSCIMGroupMemberInput
type CloudSCIMGroupSyncResult = antflycloudclient.CloudSCIMGroupSyncResult

var NewClient = antflycloudclient.NewClient

type APIError = antflycloudclient.APIError
