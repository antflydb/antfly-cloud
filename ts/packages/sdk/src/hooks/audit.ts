/**
 * Audit Log API Hooks
 *
 * React Query hooks for fetching organization audit logs.
 */

import { type UseQueryOptions, useQuery } from "@tanstack/react-query";
import { client } from "../client";
import type { components } from "../types";

// Type aliases for better readability
type AuditLog = components["schemas"]["AuditLog"];
type EntityType = components["schemas"]["EntityType"];
type AuditAction = components["schemas"]["AuditAction"];
type PaginationMeta = components["schemas"]["PaginationMeta"];

export type AuditLogsResponse = {
  data: AuditLog[];
  meta: PaginationMeta;
};

export type AuditLogsFilters = {
  offset?: number;
  limit?: number;
  entity_type?: EntityType;
  entity_id?: string;
  action?: AuditAction;
  user_id?: string;
  start_date?: string;
  end_date?: string;
};

// Query keys factory
export const auditKeys = {
  all: (orgId: string) => ["audit", orgId] as const,
  logs: (orgId: string, filters?: AuditLogsFilters) =>
    [...auditKeys.all(orgId), "logs", filters] as const,
};

/**
 * Get audit logs for an organization
 *
 * @example
 * ```tsx
 * const { data, isLoading } = useAuditLogs(orgId, {
 *   limit: 10,
 *   entity_type: "project"
 * });
 * ```
 */
export function useAuditLogs(
  orgId: string,
  filters?: AuditLogsFilters,
  options?: UseQueryOptions<AuditLogsResponse, Error>
) {
  return useQuery<AuditLogsResponse, Error>({
    queryKey: auditKeys.logs(orgId, filters),
    queryFn: async () => {
      const response = await client.GET("/organizations/{org_id}/audit-logs", {
        params: {
          path: { org_id: orgId },
          query: filters,
        },
      });

      if (response.error) {
        throw new Error(
          response.error.detail || response.error.title || "Failed to fetch audit logs"
        );
      }

      return response.data as AuditLogsResponse;
    },
    ...options,
    enabled: Boolean(orgId) && (options?.enabled ?? true),
  });
}
