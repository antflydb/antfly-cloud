import { QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CloudAPIError, client } from "../../client";
import { useOrganizationAntflyInferenceDailyCost } from "../use-antfly-inference-daily-cost";
import { useOrganizationAntflyInferenceLogs } from "../use-antfly-inference-logs";
import { useOrganizationAntflyInferenceUsageSummary } from "../use-antfly-inference-usage-summary";
import { createTestQueryClient } from "./test-utils";

describe("Antfly Inference contract-backed hooks", () => {
  const orgId = "00000000-0000-4000-8000-000000000001";
  const instanceId = "00000000-0000-4000-8000-000000000002";
  const queryClient = createTestQueryClient();
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns a populated usage response and propagates instance/model filters", async () => {
    const data = {
      organization_id: orgId,
      cloud_instance_id: instanceId,
      billing_cycle_start: "2026-07-01T00:00:00Z",
      total_requests: 1,
      total_text_tokens: 42,
      estimated_cost_usd: 0,
      unpriced_text_tokens: 42,
      unpriced_request_count: 1,
      models: [
        {
          model: "model-a",
          request_count: 1,
          text_tokens: 42,
          estimated_cost_usd: 0,
          last_seen_at: "2026-07-22T00:00:00Z",
        },
      ],
    };
    vi.spyOn(client, "GET").mockResolvedValue({
      data,
      error: undefined,
      response: new Response(JSON.stringify(data), { status: 200 }),
    } as Awaited<ReturnType<typeof client.GET>>);

    const { result } = renderHook(
      () =>
        useOrganizationAntflyInferenceUsageSummary(orgId, {
          cloudInstanceId: instanceId,
          model: "model-a",
        }),
      { wrapper }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.models).toHaveLength(1);
    expect(client.GET).toHaveBeenCalledWith(
      "/organizations/{org_id}/cloud/antfly-inference-usage-summary",
      {
        params: {
          path: { org_id: orgId },
          query: { cloud_instance_id: instanceId, model: "model-a" },
        },
      }
    );
  });

  it("treats an empty request-log response as valid data", async () => {
    const data = { data: [], meta: { total: 0, limit: 100, offset: 0 } };
    vi.spyOn(client, "GET").mockResolvedValue({
      data,
      error: undefined,
      response: new Response(JSON.stringify(data), { status: 200 }),
    } as Awaited<ReturnType<typeof client.GET>>);

    const { result } = renderHook(
      () =>
        useOrganizationAntflyInferenceLogs(orgId, {
          cloudInstanceId: instanceId,
          limit: 100,
          offset: 0,
        }),
      { wrapper }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(data);
  });

  it("exposes authorization failures separately from service failures", async () => {
    vi.spyOn(client, "GET").mockResolvedValue({
      data: undefined,
      error: { detail: "Access denied", status: 403, title: "Forbidden", type: "forbidden" },
      response: new Response(null, { status: 403 }),
    } as Awaited<ReturnType<typeof client.GET>>);

    const { result } = renderHook(() => useOrganizationAntflyInferenceUsageSummary(orgId), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(CloudAPIError);
    expect((result.current.error as CloudAPIError).kind).toBe("authorization");
    expect(result.current.error?.message).toContain("permission");
  });

  it("classifies backend failures as service errors", async () => {
    vi.spyOn(client, "GET").mockResolvedValue({
      data: undefined,
      error: {
        detail: "database unavailable",
        status: 500,
        title: "Internal Error",
        type: "internal-error",
      },
      response: new Response(null, { status: 500 }),
    } as Awaited<ReturnType<typeof client.GET>>);

    const { result } = renderHook(() => useOrganizationAntflyInferenceLogs(orgId), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(CloudAPIError);
    expect((result.current.error as CloudAPIError).kind).toBe("service");
    expect(result.current.error?.message).toContain("unavailable");
  });

  it("propagates daily-cost instance scope through the generated operation", async () => {
    const data = {
      organization_id: orgId,
      cloud_instance_id: instanceId,
      days: 14,
      daily_costs: [],
      total_cost_usd: 0,
      total_requests: 0,
    };
    vi.spyOn(client, "GET").mockResolvedValue({
      data,
      error: undefined,
      response: new Response(JSON.stringify(data), { status: 200 }),
    } as Awaited<ReturnType<typeof client.GET>>);

    const { result } = renderHook(
      () =>
        useOrganizationAntflyInferenceDailyCost(orgId, {
          days: 14,
          cloudInstanceId: instanceId,
        }),
      { wrapper }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(client.GET).toHaveBeenCalledWith(
      "/organizations/{org_id}/cloud/antfly-inference-daily-cost",
      {
        params: {
          path: { org_id: orgId },
          query: { cloud_instance_id: instanceId, days: 14 },
        },
      }
    );
  });
});
