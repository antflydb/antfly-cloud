/**
 * Invitation Acceptance API Hooks
 *
 * Provides React Query hooks for accepting invitations and managing user invitations.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";
import { InvitationAcceptanceError } from "./invitation-error";

export { InvitationAcceptanceError } from "./invitation-error";

/**
 * Hook to accept an invitation (for authenticated users)
 * POST /invitations/{token}/accept
 */
export function useAcceptInvitation(token: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client.POST("/invitations/{token}/accept", {
        params: { path: { token } },
      });

      if (error) {
        throw new InvitationAcceptanceError(response.status, error);
      }

      return data;
    },
    onSuccess: () => {
      // Invalidate organizations and user queries to refresh data
      queryClient.invalidateQueries({ queryKey: ["organizations"] });
      queryClient.invalidateQueries({ queryKey: ["user", "me"] });
      queryClient.invalidateQueries({ queryKey: ["user", "invitations"] });
    },
  });
}

/**
 * Hook to fetch pending invitations for the current user
 * GET /users/me/invitations
 */
export function usePendingInvitations() {
  return useQuery({
    queryKey: ["user", "invitations"],
    queryFn: async () => {
      const { data, error } = await client.GET("/users/me/invitations", {});

      if (error) {
        const errorMessage =
          (error as { detail?: string }).detail ||
          (error as { message?: string }).message ||
          "Failed to fetch invitations";
        throw new Error(errorMessage);
      }

      return data?.data || [];
    },
    // Refetch on window focus to stay up to date
    refetchOnWindowFocus: true,
    // Don't cache for too long (invitations are time-sensitive)
    staleTime: 1000 * 60, // 1 minute
  });
}
