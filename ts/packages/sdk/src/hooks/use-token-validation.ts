/**
 * Hook for validating tokens (invitation, verification, reset)
 *
 * Provides a unified interface for checking token validity before
 * submitting forms. Currently only supports invitation tokens
 * (verification and reset tokens are validated on submit).
 */

import { useQuery } from "@tanstack/react-query";
import { client } from "../client";
import type { operations } from "../types";

export type TokenType = "invitation" | "verification" | "reset";

// Extract InvitationDetails from the API response type
export type InvitationDetails =
  operations["getInvitation"]["responses"]["200"]["content"]["application/json"];

export interface TokenValidationError {
  type: "expired" | "invalid" | "used" | "network" | null;
  message: string | null;
}

export interface TokenValidationResult<T = unknown> {
  data: T | null;
  isLoading: boolean;
  isValid: boolean;
  error: TokenValidationError;
}

/**
 * Validate a token before accepting/using it
 *
 * @param token - The token string from the URL
 * @param type - The type of token (invitation, verification, or reset)
 * @param enabled - Whether to run the query (default: true)
 *
 * @example
 * ```tsx
 * function AcceptInvitePage({ token }: { token: string }) {
 *   const { data, isLoading, isValid, error } = useTokenValidation<InvitationDetails>(
 *     token,
 *     'invitation'
 *   );
 *
 *   if (isLoading) return <div>Validating invitation...</div>;
 *   if (!isValid) return <div>Error: {error.message}</div>;
 *
 *   return (
 *     <div>
 *       <h1>Join {data.organization_name}</h1>
 *       <p>Role: {data.role}</p>
 *     </div>
 *   );
 * }
 * ```
 */
export function useTokenValidation<T = unknown>(
  token: string,
  type: TokenType,
  enabled = true
): TokenValidationResult<T> {
  const query = useQuery({
    queryKey: [type, token],
    queryFn: async () => {
      switch (type) {
        case "invitation": {
          const { data, error, response } = await client.GET("/invitations/{token}", {
            params: { path: { token } },
          });

          if (error) {
            throw {
              message: error.detail || "Failed to validate invitation",
              response,
            };
          }

          return data;
        }

        case "verification":
          // Email verification is validated on submit, not pre-fetch
          // This prevents token consumption before user takes action
          return null;

        case "reset":
          // Password reset is validated on submit, not pre-fetch
          // This prevents token consumption before user takes action
          return null;

        default:
          throw new Error(`Unknown token type: ${type}`);
      }
    },
    enabled: enabled && !!token,
    retry: false,
    staleTime: Number.POSITIVE_INFINITY, // Token details don't change
  });

  let errorType: TokenValidationError["type"] = null;
  let errorMessage: string | null = null;

  if (query.error) {
    const apiError = query.error as {
      message?: string;
      response?: Response;
    };
    const status = apiError?.response?.status;

    switch (status) {
      case 404:
        errorType = "invalid";
        errorMessage = "This link is invalid or has been revoked.";
        break;
      case 410:
        errorType = "expired";
        errorMessage = "This link has expired. Please request a new one.";
        break;
      case 409:
        errorType = "used";
        errorMessage = "This link has already been used.";
        break;
      default:
        errorType = "network";
        errorMessage = apiError?.message || "Unable to validate link. Please try again.";
    }
  }

  return {
    data: query.data as T,
    isLoading: query.isLoading,
    isValid: !query.error && !!query.data,
    error: {
      type: errorType,
      message: errorMessage,
    },
  };
}
