/**
 * Organization management hooks using TanStack Query
 *
 * Provides hooks for creating, updating, and managing organizations.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client } from "../client";
import type { components } from "../types";

type CreateOrganizationRequest = {
  name: string;
  slug: string;
  billing_email: string;
  settings?: Record<string, unknown>;
};

type Organization = components["schemas"]["Organization"];

/**
 * Hook to create a new organization
 *
 * @example
 * ```tsx
 * const createOrg = useCreateOrganization();
 *
 * createOrg.mutate({
 *   name: "My Company",
 *   slug: "my-company",
 *   billing_email: "billing@mycompany.com"
 * }, {
 *   onSuccess: (org) => {
 *     console.log("Created org:", org.id);
 *   },
 *   onError: (error) => {
 *     console.error("Failed to create:", error.message);
 *   }
 * });
 * ```
 */
export function useCreateOrganization() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateOrganizationRequest) => {
      const {
        data: org,
        error,
        response,
      } = await client.POST("/organizations", {
        body: data,
      });

      if (error) {
        if (response?.status === 409) {
          // Use the error message from backend which includes "already exists"
          throw new Error(
            error.detail || "Organization slug already exists. Please choose a different name."
          );
        }
        throw new Error(error.detail || "Failed to create organization");
      }

      return org as Organization;
    },
    onSuccess: () => {
      // Invalidate organizations list so it refetches
      queryClient.invalidateQueries({ queryKey: ["organizations"] });
    },
  });
}

/**
 * Hook to check organization slug availability
 *
 * @example
 * ```tsx
 * const checkSlug = useCheckOrganizationSlug();
 *
 * const result = await checkSlug.mutateAsync("my-company");
 * if (result.available) {
 *   console.log("Slug is available!");
 * } else {
 *   console.log("Suggestions:", result.suggestions);
 * }
 * ```
 */
export function useCheckOrganizationSlug() {
  return useMutation({
    mutationFn: async (slug: string) => {
      const { data, error } = await client.GET("/organizations/check-slug", {
        params: {
          query: { slug },
        },
      });

      if (error) {
        throw new Error(error.detail || "Failed to check slug availability");
      }

      return data as {
        available: boolean;
        slug: string;
        suggestions?: string[];
        error?: string;
      };
    },
  });
}

/**
 * Hook to fetch organization members and pending invitations
 *
 * @param orgId - Organization ID
 * @returns Query result with members and pending invitations
 *
 * @example
 * ```tsx
 * const { data, isLoading } = useOrganizationMembers(orgId);
 *
 * if (data) {
 *   console.log("Active members:", data.data);
 *   console.log("Pending invitations:", data.pending_invitations);
 * }
 * ```
 */
export function useOrganizationMembers(orgId: string) {
  return useQuery({
    queryKey: ["organization", orgId, "members"],
    queryFn: async () => {
      const { data, error } = await client.GET("/organizations/{org_id}/members", {
        params: {
          path: { org_id: orgId },
          query: { offset: 0, limit: 100 },
        },
      });

      if (error) {
        throw new Error(error.detail || "Failed to fetch members");
      }

      return data;
    },
    enabled: !!orgId,
    staleTime: 60 * 1000, // 1 minute
  });
}

/**
 * Hook to invite a member to an organization
 *
 * @param orgId - Organization ID
 * @returns Mutation for inviting members
 *
 * @example
 * ```tsx
 * const inviteMember = useInviteMember(orgId);
 *
 * inviteMember.mutate({
 *   email: "user@example.com",
 *   role: "developer"
 * }, {
 *   onSuccess: () => {
 *     toast.success("Invitation sent!");
 *   },
 *   onError: (error) => {
 *     toast.error(error.message);
 *   }
 * });
 * ```
 */
export function useInviteMember(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { email: string; role: "admin" | "developer" }) => {
      const {
        data: result,
        error,
        response,
      } = await client.POST("/organizations/{org_id}/members", {
        params: { path: { org_id: orgId } },
        body: data,
      });

      if (error) {
        if (response?.status === 409) {
          throw new Error("User is already a member or has a pending invitation");
        }
        if (response?.status === 403) {
          throw new Error("You don't have permission to invite members");
        }
        if (response?.status === 422) {
          throw new Error("Organization member limit reached");
        }
        throw new Error(error.detail || "Failed to send invitation");
      }

      return result;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

/**
 * Hook to remove a member from an organization
 *
 * Includes protection against removing the last admin to prevent
 * locking out the organization.
 *
 * @param orgId - Organization ID
 * @returns Mutation for removing members with last admin protection
 *
 * @example
 * ```tsx
 * const removeMember = useRemoveMember(orgId);
 *
 * removeMember.mutate(
 *   { memberId: "123", memberRole: "admin", allMembers: members },
 *   {
 *     onSuccess: () => {
 *       toast.success("Member removed");
 *     },
 *     onError: (error) => {
 *       toast.error(error.message);
 *     }
 *   }
 * );
 * ```
 */
export function useRemoveMember(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (params: {
      memberId: string;
      memberRole: string;
      allMembers: Array<{ role: string }>;
    }) => {
      const { memberId, memberRole, allMembers } = params;

      // Check if removing the last admin
      if (memberRole === "admin") {
        const adminCount = allMembers.filter((m) => m.role === "admin").length;

        // If this is the only admin left (or somehow no admins), prevent removal
        if (adminCount <= 1) {
          throw new Error("Cannot remove the last admin. Promote someone else to admin first.");
        }
      }

      const { error, response } = await client.DELETE(
        "/organizations/{org_id}/members/{member_id}",
        {
          params: { path: { org_id: orgId, member_id: memberId } },
        }
      );

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to remove this member");
        }
        throw new Error(error.detail || "Failed to remove member");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

/**
 * Hook to resend an invitation
 *
 * @param orgId - Organization ID
 * @returns Mutation for resending invitations
 *
 * @example
 * ```tsx
 * const resendInvitation = useResendInvitation(orgId);
 *
 * resendInvitation.mutate(invitationToken, {
 *   onSuccess: () => {
 *     toast.success("Invitation resent!");
 *   }
 * });
 * ```
 */
export function useResendInvitation(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (token: string) => {
      const { data, error, response } = await client.POST("/invitations/{token}/resend", {
        params: { path: { token } },
      });

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to resend this invitation");
        }
        if (response?.status === 404) {
          throw new Error("Invitation not found");
        }
        throw new Error(error.detail || "Failed to resend invitation");
      }

      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

/**
 * Hook to revoke an invitation
 *
 * @param orgId - Organization ID
 * @returns Mutation for revoking invitations
 *
 * @example
 * ```tsx
 * const revokeInvitation = useRevokeInvitation(orgId);
 *
 * revokeInvitation.mutate(invitationToken, {
 *   onSuccess: () => {
 *     toast.success("Invitation revoked");
 *   }
 * });
 * ```
 */
export function useRevokeInvitation(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (token: string) => {
      const { error, response } = await client.DELETE("/invitations/{token}", {
        params: { path: { token } },
      });

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to revoke this invitation");
        }
        if (response?.status === 404) {
          throw new Error("Invitation not found");
        }
        throw new Error(error.detail || "Failed to revoke invitation");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

/**
 * Hook to update a member's role
 *
 * @param orgId - Organization ID
 * @returns Mutation for updating member roles
 *
 * @example
 * const { mutate: updateRole } = useUpdateMemberRole(orgId);
 * updateRole({ memberId: "123", role: "admin" });
 */
export function useUpdateMemberRole(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { memberId: string; role: "admin" | "developer" }) => {
      const { error, response } = await client.PATCH(
        "/organizations/{org_id}/members/{member_id}",
        {
          params: { path: { org_id: orgId, member_id: data.memberId } },
          body: { role: data.role },
        }
      );

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to update this member's role");
        }
        if (response?.status === 404) {
          throw new Error("Member not found");
        }
        throw new Error(error.detail || "Failed to update member role");
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

export function useUpdateMemberMetadata(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { memberId: string; metadata: Record<string, unknown> }) => {
      const {
        data: result,
        error,
        response,
      } = await client.PATCH("/organizations/{org_id}/members/{member_id}/metadata", {
        params: { path: { org_id: orgId, member_id: data.memberId } },
        body: { metadata: data.metadata },
      });

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to update this member's metadata");
        }
        if (response?.status === 404) {
          throw new Error("Member not found");
        }
        throw new Error(error.detail || "Failed to update member metadata");
      }

      return result;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
    },
  });
}

/**
 * Hook to transfer organization ownership
 *
 * @param orgId - Organization ID
 * @returns Mutation for transferring ownership
 *
 * @example
 * // With password
 * const { mutate: transferOwnership } = useTransferOwnership(orgId);
 * transferOwnership({ newOwnerId: "456", password: "mypassword" });
 *
 * // With OAuth re-authentication
 * transferOwnership({ newOwnerId: "456", reauthVerified: true });
 */
export function useTransferOwnership(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: {
      newOwnerId: string;
      password?: string;
      reauthVerified?: boolean;
    }) => {
      const { error, response } = await client.POST("/organizations/{org_id}/transfer-ownership", {
        params: { path: { org_id: orgId } },
        body: {
          new_owner_id: data.newOwnerId,
          password: data.password,
          reauth_verified: data.reauthVerified,
        },
      });

      if (error) {
        if (response?.status === 401) {
          throw new Error("Invalid password or re-authentication required");
        }
        if (response?.status === 403) {
          throw new Error("Only the owner can transfer ownership");
        }
        if (response?.status === 400) {
          throw new Error("New owner must be an admin or password/re-authentication required");
        }
        throw new Error(error.detail || "Failed to transfer ownership");
      }
    },
    onSuccess: () => {
      // Invalidate members query to refresh the member list with updated roles
      queryClient.invalidateQueries({ queryKey: ["organization", orgId, "members"] });
      // Also invalidate organizations list as the ownership has changed
      queryClient.invalidateQueries({ queryKey: ["organizations"] });
    },
  });
}

/**
 * Hook to update an organization's settings
 *
 * @param orgId - Organization ID
 * @returns Mutation for updating organization settings
 *
 * @example
 * ```tsx
 * const updateOrg = useUpdateOrganization(orgId);
 *
 * updateOrg.mutate({ settings: { description: "We sell widgets" } }, {
 *   onSuccess: () => toast.success("Saved!"),
 * });
 * ```
 */
export function useUpdateOrganization(orgId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: {
      name?: string;
      billing_email?: string;
      settings?: Record<string, unknown>;
    }) => {
      const {
        data: org,
        error,
        response,
      } = await client.PATCH("/organizations/{org_id}", {
        params: { path: { org_id: orgId } },
        body: data,
      });

      if (error) {
        if (response?.status === 403) {
          throw new Error("You don't have permission to update this organization");
        }
        if (response?.status === 404) {
          throw new Error("Organization not found");
        }
        throw new Error(error.detail || "Failed to update organization");
      }

      return org as Organization;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["organizations"] });
    },
  });
}

/**
 * Generate a URL-safe slug from a name
 *
 * @param name - The organization or project name
 * @returns URL-safe slug (lowercase, alphanumeric + hyphens)
 *
 * @example
 * generateSlug("My Company Inc.") // "my-company-inc"
 * generateSlug("Test@123") // "test-123"
 */
export function generateSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-") // Replace non-alphanumeric with hyphens
    .replace(/-+/g, "-") // Collapse consecutive hyphens
    .replace(/^-|-$/g, ""); // Remove leading/trailing hyphens
}
