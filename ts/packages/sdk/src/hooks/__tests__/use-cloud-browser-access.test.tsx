import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { client } from "../../client";
import {
  type CloudBrowserAccessPolicy,
  useCloudBrowserAccess,
  useUpdateCloudBrowserAccess,
} from "../use-cloud-instances";
import { createTestQueryClient } from "./test-utils";

describe("cloud browser access hooks", () => {
  const orgId = "00000000-0000-4000-8000-000000000001";
  const instanceId = "00000000-0000-4000-8000-000000000002";
  const queryClient = createTestQueryClient();
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  const queryKey = [
    "organizations",
    orgId,
    "cloud-instances",
    instanceId,
    "browser-access",
  ];
  const original: CloudBrowserAccessPolicy = {
    enabled: false,
    allowed_origins: [],
    rate_limits: {
      requests_per_minute_per_key: 60,
      requests_per_minute_per_ip: 120,
      concurrent_requests_per_key: 5,
    },
  };
  const saved: CloudBrowserAccessPolicy = {
    enabled: true,
    allowed_origins: ["https://docs.example.com"],
    rate_limits: {
      requests_per_minute_per_key: 30,
      requests_per_minute_per_ip: 60,
      concurrent_requests_per_key: 3,
    },
  };

  beforeEach(() => {
    queryClient.clear();
    queryClient.setQueryData(queryKey, original);
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("updates the policy cache from the successful server response", async () => {
    vi.spyOn(client, "PUT").mockResolvedValue({
      data: saved,
      error: undefined,
      response: new Response(JSON.stringify(saved), { status: 200 }),
    } as Awaited<ReturnType<typeof client.PUT>>);

    const { result } = renderHook(
      () => ({
        policy: useCloudBrowserAccess(orgId, instanceId),
        update: useUpdateCloudBrowserAccess(orgId, instanceId),
      }),
      { wrapper },
    );

    await act(async () => {
      await result.current.update.mutateAsync({
        enabled: saved.enabled,
        allowed_origins: saved.allowed_origins,
        rate_limits: saved.rate_limits,
      });
    });

    await waitFor(() => expect(result.current.policy.data).toEqual(saved));
    expect(queryClient.getQueryData(queryKey)).toEqual(saved);
  });
});
