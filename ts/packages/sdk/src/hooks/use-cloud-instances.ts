/**
 * Cloud instance management hooks using TanStack Query
 *
 * Provides hooks for creating, updating, deleting, and querying
 * hosted AntflyDB cloud instances.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";
import type { components } from "../types";

type CloudInstance = components["schemas"]["CloudInstance"];
type ConnectionDetails = components["schemas"]["ConnectionDetails"];
type InstanceMetrics = components["schemas"]["InstanceMetrics"];
type ProvisioningEvent = components["schemas"]["ProvisioningEvent"];
type CloudAPIKey = components["schemas"]["CloudAPIKey"];
type CloudAPIKeyCreated = components["schemas"]["CloudAPIKeyCreated"];
type PaginationMeta = components["schemas"]["PaginationMeta"];
type CloudUsageSummary = components["schemas"]["CloudUsageSummary"];
type CloudCreditEstimate = components["schemas"]["CloudCreditEstimate"];
export type CloudBillingCommitment = components["schemas"]["CloudBillingCommitment"];
export type CloudBillingCommitmentList = components["schemas"]["CloudBillingCommitmentList"];

type CreateCloudInstanceRequest = components["schemas"]["CreateCloudInstanceRequest"];
type UpdateCloudInstanceRequest = components["schemas"]["UpdateCloudInstanceRequest"];
type CreateCloudAPIKeyRequest = components["schemas"]["CreateCloudAPIKeyRequest"];
type CreateCloudManagementAPIKeyRequest =
  components["schemas"]["CreateCloudManagementAPIKeyRequest"];
type CloudCreditEstimateRequest = components["schemas"]["CloudCreditEstimateRequest"];

export type OAuthClientRecord = {
  id: string;
  client_id: string;
  name: string;
  redirect_uris: string[];
  grant_types: string[];
  response_types: string[];
  scopes: string[];
  public: boolean;
  token_endpoint_auth_method: "none" | "client_secret_post" | "client_secret_basic";
  status: "active" | "revoked";
  created_at: string;
  updated_at: string;
  last_secret_rotated_at?: string | null;
};

export type OAuthClientCreated = OAuthClientRecord & {
  client_secret?: string;
};

export type CreateOAuthClientRequest = {
  client_id?: string;
  name: string;
  redirect_uris: string[];
  scopes?: string[];
  public?: boolean;
  token_endpoint_auth_method?: "none" | "client_secret_post" | "client_secret_basic";
};

const rawAPIBaseURL = process.env.NEXT_PUBLIC_API_BASE_URL || "/api/v1";

async function fetchCloudJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const response = await fetch(`${rawAPIBaseURL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (!response.ok) {
    let message = `Request failed with status ${response.status}`;
    try {
      const body = (await response.json()) as { detail?: string; message?: string };
      message = body.detail || body.message || message;
    } catch {
      // Keep the status-based message when the response is empty or not JSON.
    }
    throw new Error(message);
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

// ── Instance CRUD ──────────────────────────────────────────────

/**
 * Hook to list cloud instances for an organization
 */
export function useCloudInstances(
  orgId: string | null,
  options?: { limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", options],
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/cloud/instances", {
        params: {
          path: { org_id: orgId },
          query: {
            limit: options?.limit,
            offset: options?.offset,
          },
        },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud instances");
      }

      return data as { data: CloudInstance[]; meta: PaginationMeta };
    },
    enabled: !!orgId,
    staleTime: 30 * 1000, // 30 seconds — instances status can change
  });
}

/**
 * Hook to list every cloud instance slug for an organization.
 */
export function useCloudInstanceSlugs(orgId: string | null, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instance-slugs"],
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");

      const limit = 100;
      let offset = 0;
      const slugs: string[] = [];

      while (true) {
        const { data, error } = await client.GET("/organizations/{org_id}/cloud/instances", {
          params: {
            path: { org_id: orgId },
            query: { limit, offset },
          },
        });

        if (error) {
          throw new Error(error.detail || "Failed to fetch cloud instance slugs");
        }

        const page = data as { data: CloudInstance[]; meta: PaginationMeta };
        const instances = page.data ?? [];
        slugs.push(...instances.map((instance) => instance.slug));

        const total = page.meta?.total;
        if (instances.length === 0 || instances.length < limit || slugs.length >= total) {
          break;
        }

        offset += instances.length;
      }

      return slugs;
    },
    enabled: !!orgId && (options?.enabled ?? true),
    staleTime: 30 * 1000,
  });
}

/**
 * Hook to get a single cloud instance
 */
export function useCloudInstance(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud instance");
      }

      return data as CloudInstance;
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 15 * 1000,
  });
}

/**
 * Hook to estimate cloud credits for a prospective cloud operation
 */
export function useCloudCreditEstimate(
  orgId: string | null | undefined,
  body: CloudCreditEstimateRequest | null,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-credit-estimate", body],
    queryFn: async (): Promise<CloudCreditEstimate> => {
      if (!orgId || !body) throw new Error("Organization and estimate request are required");

      const { data, error } = await client.POST("/organizations/{org_id}/cloud/estimate", {
        params: { path: { org_id: orgId } },
        body,
      });

      if (error) {
        throw new Error(error.detail || "Failed to estimate cloud credits");
      }

      return data;
    },
    enabled: !!orgId && !!body && (options?.enabled ?? true),
    staleTime: 30 * 1000,
  });
}

/**
 * Hook to create a cloud instance
 */
export function useCreateCloudInstance(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateCloudInstanceRequest) => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances",
        {
          params: { path: { org_id: orgId } },
          body,
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        if (response?.status === 402) {
          throw new Error("A paid billing plan is required to create cloud instances.");
        }
        if (response?.status === 409) {
          throw new Error("An instance with that slug already exists.");
        }
        throw new Error(error.detail || "Failed to create cloud instance");
      }

      return data as CloudInstance;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances"],
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instance-slugs"],
      });
    },
  });
}

/**
 * Hook to update a cloud instance
 */
export function useUpdateCloudInstance(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: UpdateCloudInstanceRequest) => {
      const { data, error, response } = await client.PATCH(
        "/organizations/{org_id}/cloud/instances/{instance_id}",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
          body,
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        if (response?.status === 404) {
          throw new Error("Cloud instance not found");
        }
        throw new Error(error.detail || "Failed to update cloud instance");
      }

      return data as CloudInstance;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId],
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances"],
        exact: false,
      });
    },
  });
}

/**
 * Hook to delete a cloud instance
 */
export function useDeleteCloudInstance(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (instanceId: string) => {
      const { error, response } = await client.DELETE(
        "/organizations/{org_id}/cloud/instances/{instance_id}",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        if (response?.status === 404) {
          throw new Error("Cloud instance not found");
        }
        throw new Error(error.detail || "Failed to delete cloud instance");
      }

      return instanceId;
    },
    onSuccess: (_data, instanceId) => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances"],
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instance-slugs"],
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId],
      });
    },
  });
}

/**
 * Hook to retry or repair provisioning for an existing cloud instance
 */
export function useProvisionCloudInstance(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances/{instance_id}/provision",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        if (response?.status === 409) {
          throw new Error(error.detail || "Instance cannot be provisioned in its current state");
        }
        if (response?.status === 404) {
          throw new Error("Cloud instance not found");
        }
        throw new Error(error.detail || "Failed to provision cloud instance");
      }

      return {
        instance: data as CloudInstance,
        queued: response.status === 202,
      };
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId],
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances"],
        exact: false,
      });
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "events"],
        exact: false,
      });
    },
  });
}

// ── Connection Details ─────────────────────────────────────────

/**
 * Hook to get connection details for a cloud instance
 */
export function useCloudInstanceConnection(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "connection"],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/connection",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch connection details");
      }

      return data as ConnectionDetails;
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 60 * 1000,
  });
}

// ── Metrics ────────────────────────────────────────────────────

/**
 * Hook to get metrics for a cloud instance
 */
export function useCloudInstanceMetrics(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "metrics"],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/metrics",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch instance metrics");
      }

      return data as InstanceMetrics;
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 30 * 1000,
  });
}

// ── Events ─────────────────────────────────────────────────────

/**
 * Hook to list provisioning events for a cloud instance
 */
export function useCloudInstanceEvents(
  orgId: string | null,
  instanceId: string | null,
  options?: { limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "events", options],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/events",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
            query: {
              limit: options?.limit,
              offset: options?.offset,
            },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch instance events");
      }

      return data as { data: ProvisioningEvent[]; meta: PaginationMeta };
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 15 * 1000,
  });
}

// ── Usage ─────────────────────────────────────────────────────

/**
 * Hook to get cloud usage for the current billing cycle
 */
export function useCloudUsage(orgId: string | undefined) {
  return useQuery({
    queryKey: ["cloud", "usage", orgId],
    queryFn: async () => {
      const { data, error } = await client.GET("/organizations/{org_id}/cloud/usage", {
        params: { path: { org_id: orgId! } },
      });
      if (error) throw new Error(error.detail || "Failed to fetch usage");
      return data as CloudUsageSummary;
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}

export function useCloudCommitments(
  orgId: string | null | undefined,
  options?: { instanceId?: string | null; limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-commitments", options],
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/cloud/commitments", {
        params: {
          path: { org_id: orgId },
          query: {
            instance_id: options?.instanceId ?? undefined,
            limit: options?.limit,
            offset: options?.offset,
          },
        },
      });

      if (error) throw new Error(error.detail || "Failed to fetch cloud commitments");
      return data as CloudBillingCommitmentList;
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}

// ── Cloud API Keys ─────────────────────────────────────────────

export function useCloudManagementAPIKeys(orgId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-management-api-keys"],
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/cloud/api-keys", {
        params: { path: { org_id: orgId } },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch CloudAF API keys");
      }

      return data as { data: CloudAPIKey[]; meta?: { total?: number } };
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}

export function useCreateCloudManagementAPIKey(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateCloudManagementAPIKeyRequest) => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/api-keys",
        {
          params: { path: { org_id: orgId } },
          body,
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        throw new Error(error.detail || "Failed to create CloudAF API key");
      }

      return data as CloudAPIKeyCreated;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-management-api-keys"],
      });
    },
  });
}

export function useRevokeCloudManagementAPIKey(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (keyId: string) => {
      const { error, response } = await client.DELETE(
        "/organizations/{org_id}/cloud/api-keys/{key_id}",
        {
          params: { path: { org_id: orgId, key_id: keyId } },
        }
      );

      if (error) {
        if (response?.status === 404) {
          throw new Error("API key not found");
        }
        throw new Error(error.detail || "Failed to revoke CloudAF API key");
      }

      return keyId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-management-api-keys"],
      });
    },
  });
}

export function useRotateCloudManagementAPIKey(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (keyId: string) => {
      const { data, error } = await client.POST(
        "/organizations/{org_id}/cloud/api-keys/{key_id}/rotate",
        {
          params: { path: { org_id: orgId, key_id: keyId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to rotate CloudAF API key");
      }

      return data as CloudAPIKeyCreated;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-management-api-keys"],
      });
    },
  });
}

// ── OAuth Clients ─────────────────────────────────────────────

const oauthClientQueryKey = (orgId: string | null | undefined) => [
  "organizations",
  orgId,
  "oauth-clients",
];

export function useOAuthClients(orgId: string | null | undefined) {
  return useQuery({
    queryKey: oauthClientQueryKey(orgId),
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");
      return fetchCloudJSON<{ data: OAuthClientRecord[]; meta?: { total?: number } }>(
        `/organizations/${encodeURIComponent(orgId)}/oauth-clients`
      );
    },
    enabled: !!orgId,
    staleTime: 60 * 1000,
  });
}

export function useCreateOAuthClient(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateOAuthClientRequest) =>
      fetchCloudJSON<OAuthClientCreated>(
        `/organizations/${encodeURIComponent(orgId)}/oauth-clients`,
        {
          method: "POST",
          body: JSON.stringify(body),
        }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: oauthClientQueryKey(orgId) });
    },
  });
}

export function useRotateOAuthClientSecret(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (clientId: string) =>
      fetchCloudJSON<OAuthClientCreated>(
        `/organizations/${encodeURIComponent(orgId)}/oauth-clients/${encodeURIComponent(
          clientId
        )}/rotate-secret`,
        { method: "POST" }
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: oauthClientQueryKey(orgId) });
    },
  });
}

export function useRevokeOAuthClient(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (clientId: string) => {
      await fetchCloudJSON<void>(
        `/organizations/${encodeURIComponent(orgId)}/oauth-clients/${encodeURIComponent(clientId)}`,
        { method: "DELETE" }
      );
      return clientId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: oauthClientQueryKey(orgId) });
    },
  });
}

/**
 * Hook to list API keys for a cloud instance
 */
export function useCloudAPIKeys(
  orgId: string | null,
  instanceId: string | null,
  options?: { limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: [
      "organizations",
      orgId,
      "cloud-instances",
      instanceId,
      "api-keys",
      { limit: options?.limit, offset: options?.offset },
    ],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      // Server supports limit/offset but OpenAPI spec doesn't declare query params yet
      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/api-keys",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
            ...(options?.limit !== undefined || options?.offset !== undefined
              ? {
                  query: {
                    limit: options?.limit,
                    offset: options?.offset,
                  },
                }
              : {}),
          } as { path: { org_id: string; instance_id: string } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud API keys");
      }

      return data as { data: CloudAPIKey[]; total: number };
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 60 * 1000,
  });
}

/**
 * Hook to create a cloud API key
 */
export function useCreateCloudAPIKey(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateCloudAPIKeyRequest) => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances/{instance_id}/api-keys",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
          body,
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        throw new Error(error.detail || "Failed to create cloud API key");
      }

      return data as CloudAPIKeyCreated;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "api-keys"],
      });
    },
  });
}

/**
 * Hook to revoke a cloud API key
 */
export function useRevokeCloudAPIKey(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (keyId: string) => {
      const { error, response } = await client.DELETE(
        "/organizations/{org_id}/cloud/instances/{instance_id}/api-keys/{key_id}",
        {
          params: {
            path: {
              org_id: orgId,
              instance_id: instanceId,
              key_id: keyId,
            },
          },
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        if (response?.status === 404) {
          throw new Error("API key not found");
        }
        throw new Error(error.detail || "Failed to revoke cloud API key");
      }

      return keyId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "api-keys"],
      });
    },
  });
}

/**
 * Hook to rotate a cloud API key
 */
export function useRotateCloudAPIKey(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (keyId: string) => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances/{instance_id}/api-keys/{key_id}/rotate",
        {
          params: {
            path: {
              org_id: orgId,
              instance_id: instanceId,
              key_id: keyId,
            },
          },
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        throw new Error(error.detail || "Failed to rotate API key");
      }

      return data as CloudAPIKeyCreated;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "api-keys"],
      });
    },
  });
}
