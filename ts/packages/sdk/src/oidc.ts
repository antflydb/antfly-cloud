/**
 * OIDC PKCE utilities for the SearchAF dashboard.
 *
 * Implements the Authorization Code flow with PKCE (S256) for authenticating
 * against the Antfly OIDC provider at auth.antfly.io.
 *
 * Configuration is loaded at runtime from the /api/config endpoint, which reads
 * server-side environment variables (OIDC_ISSUER_URL, OIDC_CLIENT_ID). This avoids
 * baking values at Next.js build time with NEXT_PUBLIC_* vars.
 */

import { PKCE_STATE_KEY, PKCE_VERIFIER_KEY } from "./constants";

// Default client ID when not specified in config
const DEFAULT_OIDC_CLIENT_ID = "searchaf-dashboard";

// Cached config fetched from /api/config
let _cachedOIDCConfig: { issuer: string | null; clientId: string } | null = null;

/**
 * Fetch OIDC configuration from the /api/config runtime endpoint.
 * Results are cached for the lifetime of the page.
 *
 * If /api/config does not include oidcIssuerUrl, issuer will be null,
 * meaning OIDC is not configured for this environment.
 */
async function fetchOIDCConfig(): Promise<{ issuer: string | null; clientId: string }> {
  if (_cachedOIDCConfig) {
    return _cachedOIDCConfig;
  }

  try {
    const response = await fetch("/api/config");
    if (response.ok) {
      const config = await response.json();
      _cachedOIDCConfig = {
        issuer: config.oidcIssuerUrl || null,
        clientId: config.oidcClientId || DEFAULT_OIDC_CLIENT_ID,
      };
      return _cachedOIDCConfig;
    }
  } catch {
    // Fall through to defaults if /api/config is unavailable
  }

  _cachedOIDCConfig = { issuer: null, clientId: DEFAULT_OIDC_CLIENT_ID };
  return _cachedOIDCConfig;
}

/**
 * Get the OIDC issuer URL if configured. Returns null when OIDC is not
 * configured for this environment (e.g., E2E tests, legacy deployments).
 */
export async function getOIDCIssuer(): Promise<string | null> {
  const config = await fetchOIDCConfig();
  return config.issuer;
}

/**
 * Reset the cached OIDC config. Intended for use in tests.
 */
export function resetOIDCConfigCache(): void {
  _cachedOIDCConfig = null;
}

/**
 * Generate a cryptographically random string for PKCE code verifier or state.
 */
function generateRandomString(length: number = 32): string {
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  return base64URLEncode(array);
}

/**
 * URL-safe base64 encoding without padding.
 */
function base64URLEncode(buffer: Uint8Array): string {
  let binary = "";
  for (const byte of buffer) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * Generate the code challenge from a code verifier using S256 method.
 */
async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return base64URLEncode(new Uint8Array(digest));
}

/**
 * Build the OIDC authorize URL and store PKCE state.
 *
 * This generates the code_verifier and state, stores them in sessionStorage,
 * and returns the full authorize URL to redirect to.
 */
export async function buildAuthorizeURL(redirectUri: string): Promise<string> {
  const { issuer, clientId } = await fetchOIDCConfig();
  if (!issuer) {
    throw new Error("OIDC is not configured — oidcIssuerUrl is not set in /api/config");
  }
  const codeVerifier = generateRandomString(32);
  const state = generateRandomString(16);
  const codeChallenge = await generateCodeChallenge(codeVerifier);

  // Store PKCE verifier and state for the callback
  sessionStorage.setItem(PKCE_VERIFIER_KEY, codeVerifier);
  sessionStorage.setItem(PKCE_STATE_KEY, state);

  const params = new URLSearchParams({
    response_type: "code",
    client_id: clientId,
    redirect_uri: redirectUri,
    scope: "openid profile email offline_access",
    state,
    code_challenge: codeChallenge,
    code_challenge_method: "S256",
  });

  return `${issuer}/authorize?${params.toString()}`;
}

export async function buildEnterpriseAwareAuthorizeURL(
  email: string,
  redirectUri: string
): Promise<string> {
  const { issuer } = await fetchOIDCConfig();
  const authorizeURL = await buildAuthorizeURL(redirectUri);
  const trimmedEmail = email.trim();
  if (!issuer || !trimmedEmail) {
    return authorizeURL;
  }

  const response = await fetch(
    `${issuer}/auth/v1/enterprise/discover?email=${encodeURIComponent(trimmedEmail)}`,
    { credentials: "include" }
  );
  if (!response.ok) {
    return authorizeURL;
  }
  const payload = (await response.json().catch(() => ({}))) as {
    data?: { provider_type?: string; login_url?: string };
  };
  if (!payload.data?.login_url || !["oidc", "saml"].includes(payload.data.provider_type ?? "")) {
    return authorizeURL;
  }
  return `${issuer}${payload.data.login_url}?return_to=${encodeURIComponent(authorizeURL)}`;
}

/**
 * Refresh an access token using a refresh token.
 */
export async function refreshAccessToken(refreshToken: string): Promise<{
  access_token: string;
  refresh_token?: string;
  expires_in: number;
  token_type: string;
}> {
  const { issuer, clientId } = await fetchOIDCConfig();
  if (!issuer) {
    throw new Error("OIDC is not configured — oidcIssuerUrl is not set in /api/config");
  }

  const response = await fetch(`${issuer}/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "refresh_token",
      refresh_token: refreshToken,
      client_id: clientId,
    }),
  });

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({}));
    throw new Error(
      (errorBody as { error_description?: string }).error_description ||
        `Token refresh failed with status ${response.status}`
    );
  }

  return response.json();
}
