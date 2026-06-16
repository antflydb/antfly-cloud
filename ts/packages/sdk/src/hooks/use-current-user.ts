/**
 * Example API hook using TanStack Query with the auto-generated API client.
 *
 * This file demonstrates how to create type-safe API hooks for the SearchAF application.
 * Copy this pattern when creating new API hooks.
 *
 * NOTE: This is a template/example.
 */

import { useQuery } from "@tanstack/react-query";
import { client } from "../client";
import type { components } from "../types";

export type User = components["schemas"]["User"];

/**
 * Fetch the current authenticated user
 *
 * @example
 * ```tsx
 * function UserProfile() {
 *   const { data: user, isLoading, error } = useCurrentUser();
 *
 *   if (isLoading) return <div>Loading...</div>;
 *   if (error) return <div>Error: {error.message}</div>;
 *   if (!user) return <div>No user found</div>;
 *
 *   return <div>Welcome, {user.display_name}!</div>;
 * }
 * ```
 */
// Helper to check if current path is a public route (no auth required)
function isPublicRoute(): boolean {
  if (typeof window === "undefined") return false;

  const pathname = window.location.pathname;
  return (
    pathname === "/login" ||
    pathname.startsWith("/auth/callback") ||
    pathname.startsWith("/accept-invitation")
  );
}

// Helper to handle auth errors (401/404) by clearing state and redirecting
async function handleAuthError(errorMessage: string): Promise<never> {
  // Only redirect if not already on a public route
  if (!isPublicRoute()) {
    localStorage.clear();

    // Clear HttpOnly auth cookies via the server-side signout route
    await fetch("/api/auth/signout", { method: "POST" }).catch(() => {
      // Best-effort — user will still be redirected to login
    });

    // Use hard redirect to force page reload and bypass middleware.
    // Include auth_error param so the login page doesn't auto-redirect
    // back to OIDC (which would cause an infinite loop).
    window.location.href = "/login?auth_error=session_expired";
  }

  throw new Error(errorMessage);
}

export function useCurrentUser() {
  // Return a disabled query during SSR to avoid QueryClient errors
  const enabled = typeof window !== "undefined";

  return useQuery({
    queryKey: ["user", "me"],
    queryFn: async () => {
      const { data, error, response } = await client.GET("/users/me");

      // Check for auth errors (401/404)
      if (response && (response.status === 404 || response.status === 401)) {
        return handleAuthError("User not found or unauthorized");
      }

      if (error) {
        throw new Error(error.detail || "Failed to fetch user");
      }

      return data;
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    refetchOnWindowFocus: true, // Refresh after returning from auth UI
    retry: false, // Don't retry on 404/401
    enabled, // Skip during SSR
  });
}

/**
 * Fetch organizations for the current user
 *
 * @example
 * ```tsx
 * function OrganizationList() {
 *   const { data, isLoading } = useOrganizations();
 *
 *   if (isLoading) return <div>Loading...</div>;
 *
 *   return (
 *     <ul>
 *       {data?.organizations?.map((org) => (
 *         <li key={org.id}>{org.name}</li>
 *       ))}
 *     </ul>
 *   );
 * }
 * ```
 */
export function useOrganizations(params?: { offset?: number; limit?: number }) {
  return useQuery({
    queryKey: ["organizations", params],
    queryFn: async () => {
      const { data, error } = await client.GET("/organizations", {
        params: {
          query: {
            offset: params?.offset ?? 0,
            limit: params?.limit ?? 20,
          },
        },
      });
      if (error) {
        throw new Error(error.detail || "Failed to fetch organizations");
      }
      return data;
    },
  });
}

/**
 * Fetch a specific organization by slug
 *
 * Uses the organizations list and finds by slug.
 * Returns the matched organization or null if not found.
 *
 * @param slug - The organization slug to find
 * @returns Query result with the organization or null
 *
 * @example
 * ```tsx
 * function OrgPage({ orgSlug }: { orgSlug: string }) {
 *   const { data: org, isLoading, error } = useOrganizationBySlug(orgSlug);
 *
 *   if (isLoading) return <div>Loading...</div>;
 *   if (!org) return <div>Organization not found</div>;
 *
 *   return <div>{org.name}</div>;
 * }
 * ```
 */
export function useOrganizationBySlug(slug: string | undefined) {
  const { data: orgsData, isLoading, error } = useOrganizations({ limit: 100 });

  const organization = slug ? (orgsData?.data?.find((org) => org.slug === slug) ?? null) : null;

  return {
    data: organization,
    isLoading,
    error,
    // Also expose the full list for redirect logic
    organizations: orgsData?.data ?? [],
  };
}
