/**
 * Invitation Acceptance API Hooks
 *
 * Provides React Query hooks for accepting invitations and managing user invitations.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";

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
        const errorMessage =
          (error as { detail?: string }).detail ||
          (error as { message?: string }).message ||
          "Failed to accept invitation";

        if (response?.status === 400) {
          throw new Error("Invalid invitation token");
        }
        if (response?.status === 403) {
          throw new Error("Invitation email does not match your account");
        }
        if (response?.status === 404) {
          throw new Error("Invitation not found or has expired");
        }
        if (response?.status === 409) {
          throw new Error("You are already a member of this organization");
        }
        throw new Error(errorMessage);
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
