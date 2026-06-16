import { useQuery } from "@tanstack/react-query";

export interface AntflyInferenceRequestLog {
  id: string;
  organization_id: string;
  cloud_instance_id?: string;
  api_key_prefix?: string;
  endpoint_path: string;
  model: string;
  text_tokens: number;
  usd_per_million_text_tokens?: number;
  estimated_cost_usd: number;
  response_status: number;
  latency_ms: number;
  created_at: string;
}

export interface AntflyInferenceRequestLogList {
  data: AntflyInferenceRequestLog[];
  meta: {
    total: number;
    limit: number;
    offset: number;
  };
}

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

      const params = new URLSearchParams();
      if (options?.limit !== undefined) params.set("limit", String(options.limit));
      if (options?.offset !== undefined) params.set("offset", String(options.offset));
      if (options?.model) params.set("model", options.model);
      if (options?.cloudInstanceId) params.set("cloud_instance_id", options.cloudInstanceId);
      const suffix = params.toString() ? `?${params.toString()}` : "";

      const response = await fetch(
        `/api/v1/organizations/${orgId}/cloud/antfly-inference-logs${suffix}`
      );
      if (!response.ok) {
        let detail = "Failed to fetch Antfly Inference request logs";
        try {
          const body = (await response.json()) as { detail?: string };
          if (body.detail) detail = body.detail;
        } catch {}
        throw new Error(detail);
      }

      return (await response.json()) as AntflyInferenceRequestLogList;
    },
    enabled: !!orgId,
    staleTime: 15 * 1000,
  });
}
