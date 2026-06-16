import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";

export type CloudGrantSubjectType = "user" | "group" | "cloud_api_key";

export type CloudGroup = {
  id: string;
  organization_id: string;
  name: string;
  slug: string;
  external_id?: string;
  description?: string;
  metadata?: Record<string, unknown>;
  created_by?: string;
  created_at: string;
  updated_at: string;
};

export type CloudGroupMember = {
  group_id: string;
  user_id: string;
  added_by?: string;
  added_at: string;
};

export type CloudGrant = {
  id: string;
  organization_id: string;
  cloud_instance_id: string;
  subject_type: CloudGrantSubjectType;
  subject_id: string;
  table_name: string;
  actions: string[];
  row_filter?: Record<string, unknown>;
  row_filter_template?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_by?: string;
  created_at: string;
  updated_at: string;
};

type CreateCloudGroupRequest = {
  name: string;
  slug?: string;
  description?: string;
  metadata?: Record<string, unknown>;
};

type UpdateCloudGroupRequest = {
  name?: string;
  description?: string;
  metadata?: Record<string, unknown>;
};

type UpsertCloudGrantRequest = {
  subject_type: CloudGrantSubjectType;
  subject_id: string;
  table_name: string;
  actions: string[];
  row_filter?: Record<string, unknown>;
  row_filter_template?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

type CloudSCIMGroupSyncRequest = {
  groups: Array<{
    external_id: string;
    display_name: string;
    slug?: string;
    description?: string;
    members: Array<{
      user_id: string;
      attributes?: Record<string, unknown>;
    }>;
  }>;
};

type CloudSCIMGroupSyncResult = {
  groups_synced: number;
  memberships_synced: number;
  attributes_synced: number;
};

export type CloudUserAttributes = {
  organization_id: string;
  user_id: string;
  synced_attributes: Record<string, unknown>;
  manual_attributes: Record<string, unknown>;
  effective_attributes: Record<string, unknown>;
  source?: string;
  updated_at?: string;
};

export function useCloudGroups(orgId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-rbac", "groups"],
    queryFn: async () => {
      if (!orgId) throw new Error("Organization ID is required");

      const { data, error } = await client.GET("/organizations/{org_id}/cloud/groups", {
        params: { path: { org_id: orgId } },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud groups");
      }

      return data as { data?: CloudGroup[]; meta?: Record<string, unknown> };
    },
    enabled: !!orgId,
    staleTime: 30 * 1000,
  });
}

export function useCreateCloudGroup(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateCloudGroupRequest) => {
      const { data, error } = await client.POST("/organizations/{org_id}/cloud/groups", {
        params: { path: { org_id: orgId } },
        body,
      });

      if (error) {
        throw new Error(error.detail || "Failed to create cloud group");
      }

      return data as CloudGroup;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "cloud-rbac"] });
    },
  });
}

export function useUpdateCloudGroup(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ groupId, body }: { groupId: string; body: UpdateCloudGroupRequest }) => {
      const { data, error } = await client.PATCH(
        "/organizations/{org_id}/cloud/groups/{group_id}",
        {
          params: { path: { org_id: orgId, group_id: groupId } },
          body,
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to update cloud group");
      }

      return data as CloudGroup;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "cloud-rbac"] });
    },
  });
}

export function useCloudUserAttributes(orgId: string | null, userId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-rbac", "users", userId, "attributes"],
    queryFn: async () => {
      if (!orgId || !userId) throw new Error("Organization and user IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/users/{user_id}/attributes",
        {
          params: { path: { org_id: orgId, user_id: userId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch user attributes");
      }

      return data as CloudUserAttributes;
    },
    enabled: !!orgId && !!userId,
    staleTime: 30 * 1000,
  });
}

export function useUpdateCloudUserAttributes(orgId: string, userId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (manualAttributes: Record<string, unknown>) => {
      const { data, error } = await client.PUT(
        "/organizations/{org_id}/cloud/users/{user_id}/attributes",
        {
          params: { path: { org_id: orgId, user_id: userId } },
          body: { manual_attributes: manualAttributes },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to update user attributes");
      }

      return data as CloudUserAttributes;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-rbac", "users", userId, "attributes"],
      });
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "cloud-rbac"] });
    },
  });
}

export function useCloudGroupMembers(orgId: string | null, groupId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-rbac", "groups", groupId, "members"],
    queryFn: async () => {
      if (!orgId || !groupId) throw new Error("Organization and group IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/groups/{group_id}/members",
        {
          params: { path: { org_id: orgId, group_id: groupId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud group members");
      }

      return data as { data?: CloudGroupMember[]; meta?: Record<string, unknown> };
    },
    enabled: !!orgId && !!groupId,
    staleTime: 30 * 1000,
  });
}

export function useAddCloudGroupMember(orgId: string, groupId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (userId: string) => {
      const { data, error } = await client.POST(
        "/organizations/{org_id}/cloud/groups/{group_id}/members",
        {
          params: { path: { org_id: orgId, group_id: groupId } },
          body: { user_id: userId },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to add cloud group member");
      }

      return data as CloudGroupMember;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-rbac", "groups", groupId, "members"],
      });
    },
  });
}

export function useRemoveCloudGroupMember(orgId: string, groupId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (userId: string) => {
      const { error } = await client.DELETE(
        "/organizations/{org_id}/cloud/groups/{group_id}/members/{user_id}",
        {
          params: { path: { org_id: orgId, group_id: groupId, user_id: userId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to remove cloud group member");
      }

      return userId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-rbac", "groups", groupId, "members"],
      });
    },
  });
}

export function useCloudGrants(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-rbac", "instances", instanceId, "grants"],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/grants",
        {
          params: { path: { org_id: orgId, instance_id: instanceId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch cloud grants");
      }

      return data as { data?: CloudGrant[]; meta?: Record<string, unknown> };
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 30 * 1000,
  });
}

export function useUpsertCloudGrant(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: UpsertCloudGrantRequest) => {
      const { data, error } = await client.PUT(
        "/organizations/{org_id}/cloud/instances/{instance_id}/grants",
        {
          params: { path: { org_id: orgId, instance_id: instanceId } },
          body,
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to save cloud grant");
      }

      return data as CloudGrant;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-rbac", "instances", instanceId, "grants"],
      });
    },
  });
}

export function useDeleteCloudGrant(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (grantId: string) => {
      const { error } = await client.DELETE(
        "/organizations/{org_id}/cloud/instances/{instance_id}/grants/{grant_id}",
        {
          params: { path: { org_id: orgId, instance_id: instanceId, grant_id: grantId } },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to delete cloud grant");
      }

      return grantId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-rbac", "instances", instanceId, "grants"],
      });
    },
  });
}

export function useSyncCloudSCIMGroups(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CloudSCIMGroupSyncRequest) => {
      const { data, error } = await client.PUT(
        "/organizations/{org_id}/cloud/scim/groups",
        {
          params: { path: { org_id: orgId } },
          body,
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to sync SCIM groups");
      }

      return data as CloudSCIMGroupSyncResult;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "cloud-rbac"] });
    },
  });
}
