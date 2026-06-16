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

var NewClient = antflycloudclient.NewClient

type APIError = antflycloudclient.APIError
