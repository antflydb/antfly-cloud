package sdk

import (
	"context"
	"github.com/antflydb/antfly-cloud/go/pkg/sdk/oapi"
	"github.com/google/uuid"
)

type CreateCloudInstanceRequest = oapi.CreateCloudInstanceRequest
type UpdateCloudInstanceRequest = oapi.UpdateCloudInstanceRequest
type CloudCreditEstimateRequest = oapi.CloudCreditEstimateRequest
type CloudCreditEstimate = oapi.CloudCreditEstimate
type NodeConfigUpdate = oapi.NodeConfigUpdate
type CloudInstanceTier = oapi.CloudInstanceTier
type CloudInstanceMode = oapi.CloudInstanceMode

func (c *Client) EstimateCloudCredits(ctx context.Context, org string, request CloudCreditEstimateRequest) (*CloudCreditEstimate, error) {
	id, err := uuid.Parse(org)
	if err != nil {
		return nil, err
	}
	response, err := c.client.EstimateCloudCreditsWithResponse(ctx, id, request)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}
func (c *Client) CreateInstance(ctx context.Context, org string, request CreateCloudInstanceRequest) (*CloudInstance, error) {
	id, err := uuid.Parse(org)
	if err != nil {
		return nil, err
	}
	response, err := c.client.CreateCloudInstanceWithResponse(ctx, id, request)
	if err != nil {
		return nil, err
	}
	if response.JSON201 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return mapCloudInstance(*response.JSON201), nil
}
func (c *Client) UpdateInstance(ctx context.Context, org, instance string, request UpdateCloudInstanceRequest) (*CloudInstance, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	response, err := c.client.UpdateCloudInstanceWithResponse(ctx, orgID, instanceID, request)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return mapCloudInstance(*response.JSON200), nil
}
