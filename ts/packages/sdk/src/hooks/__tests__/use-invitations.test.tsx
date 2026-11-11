import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { client } from "../../client";
import { InvitationAcceptanceError, useAcceptInvitation } from "../use-invitations";
import { createTestQueryClient } from "./test-utils";

describe("invitation acceptance", () => {
  it.each([
    [403, "MFA_ENROLLMENT_REQUIRED", "Set up two-factor"],
    [403, "MFA_POLICY_REQUIRED", "Sign in again"],
    [403, "Invitation email does not match authenticated user", "Invitation email does not match"],
    [
      403,
      "Invitation requires an active account with a matching verified email",
      "matching verified email",
    ],
    [403, "Authentication is no longer valid", "Authentication is no longer valid"],
    [400, "User must sign up before accepting invitation", "User must sign up"],
    [400, "Invitation is no longer valid", "no longer valid"],
    [404, "Invitation is unavailable", "unavailable"],
    [409, "You are already a member of this organization", "already a member"],
    [409, "Membership cannot be reactivated", "cannot be reactivated"],
    [403, "NEW_POLICY_REASON", "Your account cannot accept"],
    [500, undefined, "Unable to accept"],
  ])("preserves HTTP %s reason %s", async (status, detail, message) => {
    vi.spyOn(client, "POST").mockResolvedValueOnce({
      error: { title: "Request failed", detail },
      response: new Response(null, { status }),
    } as never);
    const queryClient = createTestQueryClient();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result, unmount } = renderHook(() => useAcceptInvitation("test-invitation"), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });
    await act(async () => {
      await expect(result.current.mutateAsync()).rejects.toMatchObject({
        name: "InvitationAcceptanceError",
        status,
        detail,
        title: "Request failed",
        message: expect.stringContaining(message),
      });
    });
    expect(invalidate).not.toHaveBeenCalled();
    unmount();
    queryClient.clear();
  });

  it("retains explicit machine codes and rejects malformed bodies safely", () => {
    expect(
      new InvitationAcceptanceError(403, {
        code: "MFA_POLICY_REQUIRED",
        detail: "Additional verification",
      })
    ).toMatchObject({
      code: "MFA_POLICY_REQUIRED",
      detail: "Additional verification",
      message: expect.stringContaining("Sign in again"),
    });
    for (const body of [null, "html", { detail: 42 }, { detail: "" }]) {
      expect(new InvitationAcceptanceError(403, body).message).toContain(
        "Your account cannot accept"
      );
    }
  });

  it("accepts only on mutation and refreshes membership data on success", async () => {
    const post = vi.spyOn(client, "POST").mockResolvedValueOnce({
      data: { organization_id: "org-1" },
      response: new Response(),
    } as never);
    const queryClient = createTestQueryClient();
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result, unmount } = renderHook(() => useAcceptInvitation("test-invitation"), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });
    expect(post).not.toHaveBeenCalled();
    await act(async () => {
      await result.current.mutateAsync();
    });
    expect(post).toHaveBeenCalledExactlyOnceWith("/invitations/{token}/accept", {
      params: { path: { token: "test-invitation" } },
    });
    for (const queryKey of [["organizations"], ["user", "me"], ["user", "invitations"]]) {
      expect(invalidate).toHaveBeenCalledWith({ queryKey });
    }
    unmount();
    queryClient.clear();
  });
});
