/**
 * Cloud backup management hooks using TanStack Query
 *
 * Provides hooks for creating, listing, and deleting backups,
 * triggering restores, and managing backup schedules.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";
import type { components } from "../types";

type CloudBackup = components["schemas"]["CloudBackup"];
type CloudRestore = components["schemas"]["CloudRestore"];
type CloudBackupSchedule = components["schemas"]["CloudBackupSchedule"];
type CloudBackupDownload = components["schemas"]["CloudBackupDownload"];
type CreateCloudRestoreRequest = components["schemas"]["CreateCloudRestoreRequest"];
type UpdateCloudBackupScheduleRequest = components["schemas"]["UpdateCloudBackupScheduleRequest"];

// ── Backups ────────────────────────────────────────────────────

/**
 * Hook to list backups for a cloud instance
 */
export function useCloudBackups(
  orgId: string | null,
  instanceId: string | null,
  options?: { limit?: number; offset?: number; refetchInterval?: number | false }
) {
  return useQuery({
    queryKey: [
      "organizations",
      orgId,
      "cloud-instances",
      instanceId,
      "backups",
      { limit: options?.limit, offset: options?.offset },
    ],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backups",
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
        throw new Error(error.detail || "Failed to fetch backups");
      }

      return data as { data: CloudBackup[]; total: number };
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 15 * 1000,
    refetchInterval: options?.refetchInterval,
  });
}

/**
 * Hook to get a single backup by ID
 */
export function useCloudBackup(
  orgId: string | null,
  instanceId: string | null,
  backupRecordId: string | null
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "backups", backupRecordId],
    queryFn: async () => {
      if (!orgId || !instanceId || !backupRecordId)
        throw new Error("Organization, instance, and backup IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backups/{backup_record_id}",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId, backup_record_id: backupRecordId },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch backup");
      }

      return data as CloudBackup;
    },
    enabled: !!orgId && !!instanceId && !!backupRecordId,
    staleTime: 10 * 1000,
  });
}

/**
 * Hook to create an on-demand backup
 */
export function useCreateCloudBackup(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backups",
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
        if (response?.status === 409) {
          throw new Error("A backup is already in progress.");
        }
        throw new Error(error.detail || "Failed to create backup");
      }

      return data as CloudBackup;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "backups"],
      });
    },
  });
}

/**
 * Hook to delete a backup
 */
export function useDeleteCloudBackup(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (backupRecordId: string) => {
      const { error, response } = await client.DELETE(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backups/{backup_record_id}",
        {
          params: {
            path: {
              org_id: orgId,
              instance_id: instanceId,
              backup_record_id: backupRecordId,
            },
          },
        }
      );

      if (error) {
        if (response?.status === 429) {
          throw new Error("Rate limit exceeded. Please wait a moment and try again.");
        }
        if (response?.status === 404) {
          throw new Error("Backup not found");
        }
        throw new Error(error.detail || "Failed to delete backup");
      }

      return backupRecordId;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "backups"],
      });
    },
  });
}

/**
 * Hook to get a presigned download URL for a completed backup
 */
export function useDownloadCloudBackup(orgId: string, instanceId: string) {
  return useMutation({
    mutationFn: async (backupRecordId: string) => {
      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backups/{backup_record_id}/download",
        {
          params: {
            path: {
              org_id: orgId,
              instance_id: instanceId,
              backup_record_id: backupRecordId,
            },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to get download URL");
      }

      return data as CloudBackupDownload;
    },
  });
}

// ── Restore ────────────────────────────────────────────────────

/**
 * Hook to create a restore from a backup
 */
export function useCreateCloudRestore(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: CreateCloudRestoreRequest) => {
      const { data, error, response } = await client.POST(
        "/organizations/{org_id}/cloud/instances/{instance_id}/restore",
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
        if (response?.status === 409) {
          throw new Error("A restore is already in progress.");
        }
        if (response?.status === 404) {
          throw new Error("Backup not found or not in completed state.");
        }
        throw new Error(error.detail || "Failed to create restore");
      }

      return data as CloudRestore;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId],
      });
    },
  });
}

/**
 * Hook to get a restore record by ID
 */
export function useCloudRestore(
  orgId: string | null,
  instanceId: string | null,
  restoreId: string | null
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "restores", restoreId],
    queryFn: async () => {
      if (!orgId || !instanceId || !restoreId)
        throw new Error("Organization, instance, and restore IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/restores/{restore_id}",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId, restore_id: restoreId },
          },
        }
      );

      if (error) {
        throw new Error(error.detail || "Failed to fetch restore");
      }

      return data as CloudRestore;
    },
    enabled: !!orgId && !!instanceId && !!restoreId,
    staleTime: 10 * 1000,
  });
}

/**
 * Hook to list restores for a cloud instance
 */
export function useCloudRestores(
  orgId: string | null,
  instanceId: string | null,
  options?: { limit?: number; offset?: number }
) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "restores", options],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/restores",
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
        throw new Error(error.detail || "Failed to fetch restores");
      }

      return data as { data: CloudRestore[]; total: number };
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 15 * 1000,
  });
}

// ── Backup Schedule ────────────────────────────────────────────

/**
 * Hook to get the backup schedule for an instance
 */
export function useCloudBackupSchedule(orgId: string | null, instanceId: string | null) {
  return useQuery({
    queryKey: ["organizations", orgId, "cloud-instances", instanceId, "backup-schedule"],
    queryFn: async () => {
      if (!orgId || !instanceId) throw new Error("Organization and instance IDs are required");

      const { data, error, response } = await client.GET(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backup-schedule",
        {
          params: {
            path: { org_id: orgId, instance_id: instanceId },
          },
        }
      );

      if (error) {
        // 404 means no schedule configured — return null
        if (response?.status === 404) {
          return null;
        }
        throw new Error(error.detail || "Failed to fetch backup schedule");
      }

      return data as CloudBackupSchedule;
    },
    enabled: !!orgId && !!instanceId,
    staleTime: 60 * 1000,
  });
}

/**
 * Hook to create or update the backup schedule
 */
export function useUpdateCloudBackupSchedule(orgId: string, instanceId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (body: UpdateCloudBackupScheduleRequest) => {
      const { data, error, response } = await client.PUT(
        "/organizations/{org_id}/cloud/instances/{instance_id}/backup-schedule",
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
        if (response?.status === 400) {
          throw new Error("Invalid schedule configuration.");
        }
        throw new Error(error.detail || "Failed to update backup schedule");
      }

      return data as CloudBackupSchedule;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations", orgId, "cloud-instances", instanceId, "backup-schedule"],
      });
    },
  });
}
