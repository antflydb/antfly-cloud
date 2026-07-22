import createClient from "openapi-fetch";
import type { paths } from "./types";

export type CloudAPIErrorKind = "authorization" | "request" | "service";

export class CloudAPIError extends Error {
  readonly status: number;
  readonly kind: CloudAPIErrorKind;

  constructor(message: string, status: number, kind: CloudAPIErrorKind) {
    super(message);
    this.name = "CloudAPIError";
    this.status = status;
    this.kind = kind;
  }
}

export function cloudAPIError(error: unknown, response: Response, fallback: string): CloudAPIError {
  const detail =
    typeof error === "object" && error !== null && "detail" in error
      ? String(error.detail)
      : undefined;
  if (response.status === 401 || response.status === 403) {
    return new CloudAPIError(
      "You do not have permission to view this Antfly Inference data.",
      response.status,
      "authorization"
    );
  }
  if (response.status >= 500) {
    return new CloudAPIError(fallback, response.status, "service");
  }
  return new CloudAPIError(detail || fallback, response.status, "request");
}

// Create API client with credentials support for JWT cookies
export const client = createClient<paths>({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL || "/api/v1",
  credentials: "include", // Send cookies with requests (for JWT)
  // Explicit fetch reference ensures MSW can intercept in test environment
  fetch: (req) => fetch(req),
});

export default client;
