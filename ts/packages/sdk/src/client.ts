import createClient from "openapi-fetch";
import type { paths } from "./types";

// Create API client with credentials support for JWT cookies
export const client = createClient<paths>({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL || "/api/v1",
  credentials: "include", // Send cookies with requests (for JWT)
  // Explicit fetch reference ensures MSW can intercept in test environment
  fetch: (req) => fetch(req),
});

export default client;
