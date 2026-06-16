import { useQuery } from "@tanstack/react-query";

export interface AntflyInferenceModelUsageSummaryRow {
  model: string;
  request_count: number;
  text_tokens: number;
  usd_per_million_text_tokens?: number;
  estimated_cost_usd: number;
  last_seen_at: string;
}

export interface AntflyInferenceModelUsageSummary {
  organization_id: string;
  cloud_instance_id?: string;
  model_filter?: string;
  billing_cycle_start: string;
  total_requests: number;
  total_text_tokens: number;
  estimated_cost_usd: number;
  unpriced_text_tokens: number;
  unpriced_request_count: number;
  models: AntflyInferenceModelUsageSummaryRow[];
}

export function useOrganizationAntflyInferenceUsageSummary(
  orgId: string | null,
  options?: { model?: string; cloudInstanceId?: string }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "antfly-inference-usage-summary", options],
    queryFn: async (): Promise<AntflyInferenceModelUsageSummary> => {
      if (!orgId) throw new Error("Organization ID is required");

      const params = new URLSearchParams();
      if (options?.model) params.set("model", options.model);
      if (options?.cloudInstanceId) params.set("cloud_instance_id", options.cloudInstanceId);
      const suffix = params.toString() ? `?${params.toString()}` : "";

      const response = await fetch(
        `/api/v1/organizations/${orgId}/cloud/antfly-inference-usage-summary${suffix}`
      );
      if (!response.ok) {
        let detail = "Failed to fetch Antfly Inference usage summary";
        try {
          const body = (await response.json()) as { detail?: string };
          if (body.detail) detail = body.detail;
        } catch {}
        throw new Error(detail);
      }

      return (await response.json()) as AntflyInferenceModelUsageSummary;
    },
    enabled: !!orgId,
    staleTime: 15 * 1000,
  });
}
