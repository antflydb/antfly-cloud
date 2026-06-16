import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";

const identityClient = client as {
  GET: (
    path: string,
    options?: unknown
  ) => Promise<{ data?: unknown; error?: { detail?: string } }>;
  POST: (
    path: string,
    options?: unknown
  ) => Promise<{ data?: unknown; error?: { detail?: string } }>;
  PUT: (
    path: string,
    options?: unknown
  ) => Promise<{ data?: unknown; error?: { detail?: string } }>;
  PATCH: (
    path: string,
    options?: unknown
  ) => Promise<{ data?: unknown; error?: { detail?: string } }>;
  DELETE: (
    path: string,
    options?: unknown
  ) => Promise<{ data?: unknown; error?: { detail?: string } }>;
};

export type IdentityProviderType = "oidc" | "saml";
export type IdentityProviderStatus = "draft" | "verified" | "enabled" | "disabled";

export type IdentityProvider = {
  id: string;
  organization_id: string;
  provider_type: IdentityProviderType;
  display_name: string;
  status: IdentityProviderStatus;
  config: Record<string, unknown>;
  jit_enabled: boolean;
  default_member_role: "admin" | "developer";
  verified_at?: string;
  verification_error?: string;
  created_at: string;
  updated_at: string;
};

export type UpsertIdentityProviderRequest = {
  provider_type?: IdentityProviderType;
  display_name: string;
  status: IdentityProviderStatus;
  config: Record<string, unknown>;
  client_secret?: string;
  jit_enabled: boolean;
  default_member_role: "admin" | "developer";
};

export type VerifiedDomain = {
  id: string;
  organization_id: string;
  domain: string;
  verification_token: string;
  verification_method: string;
  status: "pending" | "verified" | "disabled";
  verified_at?: string;
  last_checked_at?: string;
  verification_error?: string;
  created_at: string;
  updated_at: string;
};

export type SCIMToken = {
  id: string;
  organization_id: string;
  name: string;
  token_prefix: string;
  status: "active" | "revoked";
  last_used_at?: string;
  created_at: string;
  revoked_at?: string;
};

export type CreateSCIMTokenResponse = SCIMToken & { token: string };

export type IdentityGroupMapping = {
  id: string;
  organization_id: string;
  external_group_id: string;
  external_group_name?: string;
  org_role?: "admin" | "developer" | "";
  cloud_group_id?: string;
  created_at: string;
  updated_at: string;
};

export function useIdentityProviders(orgId: string | undefined) {
  return useQuery({
    queryKey: ["organizations", orgId, "identity", "providers"],
    enabled: !!orgId,
    queryFn: async () => {
      const { data, error } = await identityClient.GET(
        "/organizations/{org_id}/identity/providers",
        { params: { path: { org_id: orgId } } }
      );
      if (error) throw new Error(error.detail || "Failed to load identity providers");
      return data as { data: IdentityProvider[] };
    },
  });
}

export function useCreateIdentityProvider(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: UpsertIdentityProviderRequest & { provider_type: IdentityProviderType }
    ) => {
      const { data, error } = await identityClient.POST(
        "/organizations/{org_id}/identity/providers",
        { params: { path: { org_id: orgId } }, body }
      );
      if (error) throw new Error(error.detail || "Failed to create identity provider");
      return data as IdentityProvider;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useUpdateIdentityProvider(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      providerId,
      body,
    }: {
      providerId: string;
      body: UpsertIdentityProviderRequest;
    }) => {
      const { data, error } = await identityClient.PATCH(
        "/organizations/{org_id}/identity/providers/{provider_id}",
        { params: { path: { org_id: orgId, provider_id: providerId } }, body }
      );
      if (error) throw new Error(error.detail || "Failed to update identity provider");
      return data as IdentityProvider;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useVerifyIdentityProvider(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (providerId: string) => {
      const { data, error } = await identityClient.POST(
        "/organizations/{org_id}/identity/providers/{provider_id}/verify",
        { params: { path: { org_id: orgId, provider_id: providerId } } }
      );
      if (error) throw new Error(error.detail || "Failed to verify identity provider");
      return data as IdentityProvider;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useVerifiedDomains(orgId: string | undefined) {
  return useQuery({
    queryKey: ["organizations", orgId, "identity", "domains"],
    enabled: !!orgId,
    queryFn: async () => {
      const { data, error } = await identityClient.GET("/organizations/{org_id}/identity/domains", {
        params: { path: { org_id: orgId } },
      });
      if (error) throw new Error(error.detail || "Failed to load domains");
      return data as { data: VerifiedDomain[] };
    },
  });
}

export function useCreateVerifiedDomain(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (domain: string) => {
      const { data, error } = await identityClient.POST(
        "/organizations/{org_id}/identity/domains",
        {
          params: { path: { org_id: orgId } },
          body: { domain },
        }
      );
      if (error) throw new Error(error.detail || "Failed to add domain");
      return data as VerifiedDomain;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useVerifyDomain(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (domainId: string) => {
      const { data, error } = await identityClient.POST(
        "/organizations/{org_id}/identity/domains/{domain_id}/verify",
        { params: { path: { org_id: orgId, domain_id: domainId } } }
      );
      if (error) throw new Error(error.detail || "Failed to verify domain");
      return data as VerifiedDomain;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useSCIMTokens(orgId: string | undefined) {
  return useQuery({
    queryKey: ["organizations", orgId, "identity", "scim", "tokens"],
    enabled: !!orgId,
    queryFn: async () => {
      const { data, error } = await identityClient.GET(
        "/organizations/{org_id}/identity/scim/tokens",
        { params: { path: { org_id: orgId } } }
      );
      if (error) throw new Error(error.detail || "Failed to load SCIM tokens");
      return data as { data: SCIMToken[] };
    },
  });
}

export function useCreateSCIMToken(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => {
      const { data, error } = await identityClient.POST(
        "/organizations/{org_id}/identity/scim/tokens",
        { params: { path: { org_id: orgId } }, body: { name } }
      );
      if (error) throw new Error(error.detail || "Failed to create SCIM token");
      return data as CreateSCIMTokenResponse;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useRevokeSCIMToken(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (tokenId: string) => {
      const { error } = await identityClient.DELETE(
        "/organizations/{org_id}/identity/scim/tokens/{token_id}",
        { params: { path: { org_id: orgId, token_id: tokenId } } }
      );
      if (error) throw new Error(error.detail || "Failed to revoke SCIM token");
      return tokenId;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}

export function useIdentityGroupMappings(orgId: string | undefined) {
  return useQuery({
    queryKey: ["organizations", orgId, "identity", "group-mappings"],
    enabled: !!orgId,
    queryFn: async () => {
      const { data, error } = await identityClient.GET(
        "/organizations/{org_id}/identity/group-mappings",
        { params: { path: { org_id: orgId } } }
      );
      if (error) throw new Error(error.detail || "Failed to load group mappings");
      return data as { data: IdentityGroupMapping[] };
    },
  });
}

export function useUpsertIdentityGroupMapping(orgId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      external_group_id: string;
      external_group_name?: string;
      org_role?: string;
      cloud_group_id?: string;
    }) => {
      const { data, error } = await identityClient.PUT(
        "/organizations/{org_id}/identity/group-mappings",
        { params: { path: { org_id: orgId } }, body }
      );
      if (error) throw new Error(error.detail || "Failed to save group mapping");
      return data as IdentityGroupMapping;
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "identity"] }),
  });
}
