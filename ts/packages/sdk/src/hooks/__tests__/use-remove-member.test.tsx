import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { client } from "../../client";
import { useRemoveMember } from "../use-organizations";
import { createTestQueryClient } from "./test-utils";

/**
 * Unit tests for useRemoveMember hook
 *
 * Tests the last admin protection logic that prevents removing
 * the last admin from an organization.
 */

describe("useRemoveMember - Last Admin Protection", () => {
  const orgId = "org-123";
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = createTestQueryClient();
    vi.clearAllMocks();

    // Mock successful DELETE by default
    vi.spyOn(client, "DELETE").mockResolvedValue({
      data: undefined,
      error: undefined,
      response: new Response(null, { status: 204 }),
    } as Awaited<ReturnType<typeof client.DELETE>>);
  });

  afterEach(() => {
    queryClient.clear();
    vi.restoreAllMocks();
  });

  it("should prevent removing the last admin", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    // Create a member list with only 1 admin
    const members = [
      { role: "owner" },
      { role: "admin" }, // Only admin - should be protected
      { role: "developer" },
      { role: "developer" },
    ];

    const adminMemberId = "member-admin-123";

    // Try to remove the only admin
    let error: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({
          memberId: adminMemberId,
          memberRole: "admin",
          allMembers: members,
        });
      } catch (e) {
        error = e;
      }
    });

    expect(error).toBeDefined();
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toBe(
      "Cannot remove the last admin. Promote someone else to admin first."
    );
  });

  it("should allow removing an admin when there are multiple admins", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    // Create a member list with 2 admins
    const members = [
      { role: "owner" },
      { role: "admin" }, // First admin
      { role: "admin" }, // Second admin - ok to remove one
      { role: "developer" },
    ];

    const adminMemberId = "member-admin-123";

    // Should succeed - no error thrown
    await act(async () => {
      await result.current.mutateAsync({
        memberId: adminMemberId,
        memberRole: "admin",
        allMembers: members,
      });
    });

    // Verify the DELETE was called
    expect(client.DELETE).toHaveBeenCalledWith("/organizations/{org_id}/members/{member_id}", {
      params: { path: { org_id: orgId, member_id: adminMemberId } },
    });
  });

  it("should allow removing a developer (no admin protection)", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    // Create a member list with 1 admin and multiple developers
    const members = [
      { role: "owner" },
      { role: "admin" },
      { role: "developer" }, // Removing developer is always ok
      { role: "developer" },
    ];

    const developerMemberId = "member-dev-123";

    // Should succeed - developers can always be removed
    await act(async () => {
      await result.current.mutateAsync({
        memberId: developerMemberId,
        memberRole: "developer",
        allMembers: members,
      });
    });

    // Verify the DELETE was called
    expect(client.DELETE).toHaveBeenCalledWith("/organizations/{org_id}/members/{member_id}", {
      params: { path: { org_id: orgId, member_id: developerMemberId } },
    });
  });

  it("should count admins correctly (excluding owner)", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    // Create a member list with owner + 1 admin
    const members = [
      { role: "owner" }, // Owner doesn't count as admin for this check
      { role: "admin" }, // Only 1 admin role
      { role: "developer" },
    ];

    const adminMemberId = "member-admin-123";

    // Try to remove the only admin (even though owner exists)
    let error: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({
          memberId: adminMemberId,
          memberRole: "admin",
          allMembers: members,
        });
      } catch (e) {
        error = e;
      }
    });

    expect(error).toBeDefined();
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toBe(
      "Cannot remove the last admin. Promote someone else to admin first."
    );
  });

  it("should handle empty member list gracefully", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    const members: Array<{ role: string }> = [];
    const adminMemberId = "member-admin-123";

    // Empty member list - should block removal
    let error: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({
          memberId: adminMemberId,
          memberRole: "admin",
          allMembers: members,
        });
      } catch (e) {
        error = e;
      }
    });

    expect(error).toBeDefined();
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toBe(
      "Cannot remove the last admin. Promote someone else to admin first."
    );
  });

  it("should handle removal attempt with admin count of zero", async () => {
    const { result } = renderHook(() => useRemoveMember(orgId), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    // Member list with no admins (edge case)
    const members = [{ role: "owner" }, { role: "developer" }, { role: "developer" }];

    const adminMemberId = "member-admin-123";

    // Try to remove when admin count is already 0
    // This shouldn't happen in practice, but tests the logic
    let error: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({
          memberId: adminMemberId,
          memberRole: "admin",
          allMembers: members,
        });
      } catch (e) {
        error = e;
      }
    });

    expect(error).toBeDefined();
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toBe(
      "Cannot remove the last admin. Promote someone else to admin first."
    );
  });
});
