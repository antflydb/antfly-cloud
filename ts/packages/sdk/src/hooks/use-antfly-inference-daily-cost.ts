import { useQuery } from "@tanstack/react-query";
import { client, cloudAPIError } from "../client";
import type { components } from "../types";

export type AntflyInferenceDailyCostEntry = components["schemas"]["AntflyInferenceDailyCostEntry"];
export type AntflyInferenceDailyCostSummary =
  components["schemas"]["AntflyInferenceDailyCostSummary"];

export function useOrganizationAntflyInferenceDailyCost(
  orgId: string | null,
  options?: { days?: number; cloudInstanceId?: string }
) {
  const days = options?.days ?? 7;
  return useQuery({
    queryKey: [
      "organizations",
      orgId,
      "antfly-inference-daily-cost",
      { days, cloudInstanceId: options?.cloudInstanceId },
    ],
    queryFn: async (): Promise<AntflyInferenceDailyCostSummary> => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error, response } = await client.GET(
        "/organizations/{org_id}/cloud/antfly-inference-daily-cost",
        {
          params: {
            path: { org_id: orgId },
            query: { days, cloud_instance_id: options?.cloudInstanceId },
          },
        }
      );
      if (error) {
        throw cloudAPIError(error, response, "Antfly Inference daily cost is unavailable.");
      }
      return data;
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}
