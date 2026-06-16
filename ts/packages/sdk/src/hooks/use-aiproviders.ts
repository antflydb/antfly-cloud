/**
 * Deprecated organization-level AI provider hooks.
 *
 * Cloud v1 provider credentials are scoped to a single cloud instance; use the
 * instance credential UI/API instead of organization-level provider config.
 */

import { useMutation, useQuery } from "@tanstack/react-query";

type AIProvider = "gemini" | "openai" | "anthropic" | "openai_compatible";
type ProviderConfigProvider = AIProvider | "s3";

type ProviderConfig = {
  id: string;
  organization_id: string;
  provider: ProviderConfigProvider;
  enabled: boolean;
  has_api_key?: boolean;
  config?: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
};

type ProviderConfigCreate = {
  provider: ProviderConfigProvider;
  api_key?: string;
  config?: Record<string, unknown> | null;
  enabled: boolean;
};

type ProviderConfigUpdate = {
  api_key?: string | null;
  config?: Record<string, unknown> | null;
  enabled?: boolean;
};

const removedError = () =>
  new Error(
    "Organization-level AI provider configuration is no longer supported; configure provider keys on a cloud instance instead."
  );

export function useAIProviderConfigs(_orgId: string | null) {
  return useQuery<ProviderConfig[]>({
    queryKey: ["org-provider-configs-removed"],
    queryFn: async () => {
      throw removedError();
    },
    enabled: false,
  });
}

export function useCreateAIProviderConfig(_orgId: string) {
  return useMutation<ProviderConfig, Error, ProviderConfigCreate>({
    mutationFn: async () => {
      throw removedError();
    },
  });
}

export function useUpdateAIProviderConfig(_orgId: string) {
  return useMutation<
    ProviderConfig,
    Error,
    { provider: AIProvider; updates: ProviderConfigUpdate }
  >({
    mutationFn: async () => {
      throw removedError();
    },
  });
}

export function useDeleteAIProviderConfig(_orgId: string) {
  return useMutation<void, Error, AIProvider>({
    mutationFn: async () => {
      throw removedError();
    },
  });
}

export type { AIProvider, ProviderConfig, ProviderConfigCreate, ProviderConfigUpdate };
