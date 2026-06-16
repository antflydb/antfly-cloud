import { useQuery } from "@tanstack/react-query";

export interface MetricPanel {
  id: string;
  title: string;
  unit: string;
  group_by?: string | null;
}

export interface MetricPoint {
  t: string;
  v: number | null;
}

export interface MetricSeries {
  labels: Record<string, string>;
  points: MetricPoint[];
}

export interface MetricQueryResponse {
  panel_id: string;
  unit: string;
  group_by: string | null;
  range: {
    from: string;
    to: string;
    step: string;
  };
  series: MetricSeries[];
}

export interface MetricRange {
  from: string;
  to: string;
  step?: string;
}

async function readError(response: Response, fallback: string): Promise<string> {
  try {
    const body = (await response.json()) as { detail?: string; message?: string };
    return body.detail || body.message || fallback;
  } catch {
    return fallback;
  }
}

export function useCloudInstanceMetricPanels(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud", "instances", instanceId, "metrics", "panels"],
    queryFn: async (): Promise<MetricPanel[]> => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const response = await fetch(
        `/api/v1/organizations/${orgId}/cloud/instances/${instanceId}/metrics/panels`
      );
      if (!response.ok) {
        throw new Error(await readError(response, "Failed to fetch metric panels"));
      }
      const body = (await response.json()) as { panels: MetricPanel[] };
      return body.panels;
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 30_000,
  });
}

export function useCloudInstanceMetricQuery(
  orgId: string | null,
  instanceId: string | null,
  options: { panel: string; range: MetricRange; enabled?: boolean }
) {
  return useQuery({
    queryKey: [
      "organizations",
      orgId,
      "cloud",
      "instances",
      instanceId,
      "metrics",
      options.panel,
      options.range,
    ],
    queryFn: async (): Promise<MetricQueryResponse> => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const params = new URLSearchParams({
        panel: options.panel,
        from: options.range.from,
        to: options.range.to,
      });
      if (options.range.step) params.set("step", options.range.step);

      const response = await fetch(
        `/api/v1/organizations/${orgId}/cloud/instances/${instanceId}/metrics/query?${params}`
      );
      if (!response.ok) {
        throw new Error(await readError(response, "Failed to fetch metric data"));
      }
      return (await response.json()) as MetricQueryResponse;
    },
    enabled: !!orgId && !!instanceId && !!options.panel && options.enabled !== false,
    staleTime: 30_000,
  });
}
