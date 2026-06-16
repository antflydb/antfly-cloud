import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { client } from "../client";
import type { components } from "../types";

// Types for profile update
export type UpdateProfileInput = {
  display_name?: string;
  avatar_url?: string | null;
};

type User = components["schemas"]["User"];

/**
 * Updates the current user's profile information
 */
export function useUpdateProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (input: UpdateProfileInput): Promise<User> => {
      const { data, error } = await client.PATCH("/users/me", {
        body: input,
      });

      if (error) {
        const errorMessage =
          typeof error === "object" && error !== null && "detail" in error
            ? String(error.detail)
            : "Failed to update profile";
        throw new Error(errorMessage);
      }

      return data;
    },
    onSuccess: () => {
      // Invalidate the current user query to refetch fresh data
      queryClient.invalidateQueries({ queryKey: ["current-user"] });
      toast.success("Profile updated successfully");
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Failed to update profile");
    },
  });
}
