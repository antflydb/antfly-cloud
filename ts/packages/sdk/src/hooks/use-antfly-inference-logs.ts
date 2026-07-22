import { useQuery } from "@tanstack/react-query";
import { client, cloudAPIError } from "../client";
import type { components } from "../types";

export type AntflyInferenceRequestLog = components["schemas"]["AntflyInferenceRequestLog"];
export type AntflyInferenceRequestLogList = components["schemas"]["AntflyInferenceRequestLogList"];

export function useOrganizationAntflyInferenceLogs(
  orgId: string | null,
  options?: {
    limit?: number;
    offset?: number;
    model?: string;
    cloudInstanceId?: string;
  }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "antfly-inference-logs", options],
    queryFn: async (): Promise<AntflyInferenceRequestLogList> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error, response } = await client.GET(
        "/organizations/{org_id}/cloud/antfly-inference-logs",
        {
          params: {
            path: { org_id: orgId },
            query: {
              limit: options?.limit,
              offset: options?.offset,
              model: options?.model,
              cloud_instance_id: options?.cloudInstanceId,
            },
          },
        }
      );
      if (error) {
        throw cloudAPIError(error, response, "Antfly Inference request logs are unavailable.");
      }
      return data;
    },
    enabled: !!orgId,
    staleTime: 15 * 1000,
  });
}
