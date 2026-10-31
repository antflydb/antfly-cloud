package sdk

import (
	"context"
	"fmt"
	"github.com/antflydb/antfly-cloud/go/pkg/sdk/oapi"
)

type LiteOperation = oapi.LiteTransferOperation
type LiteCapabilities = oapi.CloudCapabilities
type LiteUploadSession = oapi.LiteUploadSession
type LiteDownload = oapi.LiteDownload
type StartLiteImportRequest = oapi.StartLiteImportRequest
type ConfirmLiteImportRequest = oapi.ConfirmLiteImportRequest
type CreateInstanceRequest = oapi.CreateCloudInstanceRequest

func (c *Client) Capabilities(ctx context.Context, org string) (*LiteCapabilities, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	response, err := c.client.GetCloudCapabilitiesWithResponse(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

func (c *Client) StartLiteImport(ctx context.Context, org string, request StartLiteImportRequest) (*LiteOperation, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	response, err := c.client.StartLiteImportWithResponse(ctx, orgID, request)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

func (c *Client) LiteOperation(ctx context.Context, org string, operation string) (*LiteOperation, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.GetLiteOperationWithResponse(ctx, orgID, operationID)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

func (c *Client) LiteUploadSession(ctx context.Context, org string, operation string) (*LiteUploadSession, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.CreateLiteUploadSessionWithResponse(ctx, orgID, operationID)
	if err != nil {
		return nil, fmt.Errorf("scoped transfer URL request failed")
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("scoped transfer URL request returned HTTP %d", response.StatusCode())
	}
	return response.JSON200, nil
}

func (c *Client) SealLiteUpload(ctx context.Context, org string, operation string) (*LiteOperation, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.SealLiteUploadWithResponse(ctx, orgID, operationID)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

func (c *Client) CancelLiteOperation(ctx context.Context, org string, operation string) (*LiteOperation, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.CancelLiteOperationWithResponse(ctx, orgID, operationID)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

func (c *Client) ConfirmLiteImport(ctx context.Context, org string, operation string, request ConfirmLiteImportRequest) (*CloudInstance, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.ConfirmLiteImportWithResponse(ctx, orgID, operationID, request)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	mapped := mapCloudInstance(*response.JSON200)
	return &mapped, nil
}

func (c *Client) LiteDownload(ctx context.Context, org string, operation string) (*LiteDownload, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	operationID, err := parseUUID(operation, "operation")
	if err != nil {
		return nil, err
	}
	response, err := c.client.CreateLiteDownloadWithResponse(ctx, orgID, operationID)
	if err != nil {
		return nil, fmt.Errorf("scoped transfer URL request failed")
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("scoped transfer URL request returned HTTP %d", response.StatusCode())
	}
	return response.JSON200, nil
}

func (c *Client) CreateInstance(ctx context.Context, org string, request CreateInstanceRequest) (*CloudInstance, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	response, err := c.client.CreateCloudInstanceWithResponse(ctx, orgID, request)
	if err != nil {
		return nil, err
	}
	if response.JSON201 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	mapped := mapCloudInstance(*response.JSON201)
	return &mapped, nil
}

func (c *Client) StartLiteExport(ctx context.Context, org, instance, key string) (*LiteOperation, error) {
	orgID, instanceID, err := parseOrgInstance(org, instance)
	if err != nil {
		return nil, err
	}
	response, err := c.client.StartLiteExportWithResponse(ctx, orgID, instanceID, oapi.StartLiteExportRequest{IdempotencyKey: key})
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	return response.JSON200, nil
}

// ResumeLite resumes the retained owner. Poll GetInstance until ready before
// retrying a query or insert; no failed write is replayed by this operation.
func (c *Client) ResumeLite(ctx context.Context, org, instance string) (*CloudInstance, error) {
	orgID, err := parseUUID(org, "org")
	if err != nil {
		return nil, err
	}
	id, err := parseUUID(instance, "instance")
	if err != nil {
		return nil, err
	}
	response, err := c.client.ResumeLiteInstanceWithResponse(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if response.JSON202 == nil {
		return nil, apiError(response.StatusCode(), response.Body)
	}
	mapped := mapCloudInstance(*response.JSON202)
	return &mapped, nil
}
