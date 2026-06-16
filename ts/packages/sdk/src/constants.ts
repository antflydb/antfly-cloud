/**
 * Auth-related cookie names.
 *
 * Shared between @antfly/cloud-sdk (use-sign-out, etc.) and the dashboard app
 * so cookie names are defined in one place.
 */

export const COOKIE_ACCESS_TOKEN = "access_token";
export const COOKIE_REFRESH_TOKEN = "refresh_token";

/**
 * PKCE sessionStorage keys used during the OIDC Authorization Code flow.
 *
 * Shared between oidc.ts (writes) and the auth callback page (reads).
 */
export const PKCE_VERIFIER_KEY = "oidc_code_verifier";
export const PKCE_STATE_KEY = "oidc_state";
