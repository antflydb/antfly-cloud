import { useQuery } from "@tanstack/react-query";
import { client, cloudAPIError } from "../client";
import type { components } from "../types";

export type AntflyInferenceModelUsageSummaryRow =
  components["schemas"]["AntflyInferenceModelUsageSummaryRow"];
export type AntflyInferenceModelUsageSummary =
  components["schemas"]["AntflyInferenceModelUsageSummary"];

export function useOrganizationAntflyInferenceUsageSummary(
  orgId: string | null,
  options?: { model?: string; cloudInstanceId?: string }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "antfly-inference-usage-summary", options],
    queryFn: async (): Promise<AntflyInferenceModelUsageSummary> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error, response } = await client.GET(
        "/organizations/{org_id}/cloud/antfly-inference-usage-summary",
        {
          params: {
            path: { org_id: orgId },
            query: {
              model: options?.model,
              cloud_instance_id: options?.cloudInstanceId,
            },
          },
        }
      );
      if (error) {
        throw cloudAPIError(error, response, "Antfly Inference usage summary is unavailable.");
      }
      return data;
    },
    enabled: !!orgId,
    staleTime: 15 * 1000,
  });
}
