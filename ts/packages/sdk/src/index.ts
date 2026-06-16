import createClient from "openapi-fetch";
import type { paths } from "./types.js";

export type AntflyCloudClientOptions = Parameters<typeof createClient<paths>>[0];

export function createAntflyCloudClient(options: AntflyCloudClientOptions = {}) {
  return createClient<paths>({
    baseUrl: "https://cloud.antfly.io/api/v1",
    ...options
  });
}

export type { paths };
