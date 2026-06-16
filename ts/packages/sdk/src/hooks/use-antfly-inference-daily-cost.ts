import { useQuery } from "@tanstack/react-query";

export interface AntflyInferenceDailyCostEntry {
  date: string;
  total_cost_usd: number;
  total_requests: number;
  total_text_tokens: number;
}

export interface AntflyInferenceDailyCostSummary {
  organization_id: string;
  days: number;
  daily_costs: AntflyInferenceDailyCostEntry[];
  total_cost_usd: number;
  total_requests: number;
}

export function useOrganizationAntflyInferenceDailyCost(
  orgId: string | null,
  options?: { days?: number }
) {
  const days = options?.days ?? 7;
  return useQuery({
    queryKey: ["organizations", orgId, "antfly-inference-daily-cost", { days }],
    queryFn: async (): Promise<AntflyInferenceDailyCostSummary> => {
      if (!orgId) throw new Error("Organization ID is required");

      const response = await fetch(
        `/api/v1/organizations/${orgId}/cloud/antfly-inference-daily-cost?days=${days}`
      );
      if (!response.ok) {
        let detail = "Failed to fetch Antfly Inference daily cost";
        try {
          const body = (await response.json()) as { detail?: string };
          if (body.detail) detail = body.detail;
        } catch {}
        throw new Error(detail);
      }

      return (await response.json()) as AntflyInferenceDailyCostSummary;
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}
