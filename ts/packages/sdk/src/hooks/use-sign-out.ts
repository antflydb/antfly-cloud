"use client";

import { useMutation } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { client } from "../client";
import { getOIDCIssuer } from "../oidc";

/**
 * Clear auth cookies via the server-side /api/auth/signout route.
 * HttpOnly cookies cannot be cleared by client-side JS.
 */
async function clearAuthCookies() {
  try {
    await fetch("/api/auth/signout", { method: "POST" });
  } catch {
    // Best-effort — user will still be redirected to login
  }
}

/**
 * Keys to preserve in localStorage during logout.
 * These are user preferences that should persist across sessions.
 */
const PRESERVE_KEYS = [
  "searchaf_org_id",
  "searchaf_org_slug",
  "searchaf_project_id",
  "searchaf_app_id",
  "searchaf-theme",
] as const;

/**
 * Clear localStorage while preserving user preferences.
 * On localhost we clear all auth-related storage except theme to avoid stale
 * dev context; on hosted environments we retain org/project preferences.
 */
function clearAuthLocalStorage() {
  const isLocalhost = typeof window !== "undefined" && window.location.hostname === "localhost";
  const keysToPreserve = isLocalhost ? ["searchaf-theme"] : PRESERVE_KEYS;

  // Save values for keys we want to preserve
  const preserved: Record<string, string | null> = {};
  for (const key of keysToPreserve) {
    preserved[key] = localStorage.getItem(key);
  }

  // Clear all localStorage
  localStorage.clear();

  // Restore preserved values
  for (const key of keysToPreserve) {
    const value = preserved[key];
    if (value != null) {
      localStorage.setItem(key, value);
    }
  }
}

/**
 * useSignOut Hook
 *
 * Handles user sign out:
 * 1. Calls backend /auth/signout (best-effort, to invalidate server session)
 * 2. Clears HttpOnly auth cookies via /api/auth/signout
 * 3. Clears localStorage while preserving user preferences (org, project, theme)
 * 4. Redirects to OIDC provider logout (or /login if OIDC is not configured)
 */
export function useSignOut() {
  const router = useRouter();

  const mutation = useMutation({
    mutationFn: async () => {
      // Best-effort backend session invalidation — ignore failures
      await client.POST("/auth/signout", {}).catch(() => {});

      // Clear HttpOnly auth cookies via server route
      await clearAuthCookies();

      // Clear auth-related localStorage while preserving user preferences
      clearAuthLocalStorage();

      toast.success("Successfully signed out");

      // Redirect to the OIDC provider's logout endpoint to destroy the SSO session,
      // with a post_logout_redirect_uri back to the dashboard login page.
      // If OIDC is not configured, fall back to local /login redirect.
      const issuer = await getOIDCIssuer();
      if (issuer) {
        const postLogoutRedirect = `${window.location.origin}/login`;
        window.location.href = `${issuer}/logout?post_logout_redirect_uri=${encodeURIComponent(postLogoutRedirect)}`;
      } else {
        router.push("/login");
      }
    },
  });

  return {
    signOut: mutation.mutate,
    isLoading: mutation.isPending,
    isPending: mutation.isPending,
  };
}
