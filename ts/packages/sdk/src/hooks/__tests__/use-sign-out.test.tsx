import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { client } from "../../client";
import { useSignOut } from "../use-sign-out";
import { createTestQueryClient, setupMockLocalStorage } from "./test-utils";

/**
 * Tests for useSignOut hook
 *
 * Covers:
 * - localStorage clearing on sign out
 * - Redirect to /login after sign out
 * - Success toast notification
 * - API endpoint call
 * - Graceful handling of API failures
 */

// Mock sonner toast
vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

// Mock OIDC module — return null (OIDC not configured) so the hook falls back to router.push("/login")
vi.mock("../../oidc", () => ({
  getOIDCIssuer: vi.fn().mockResolvedValue(null),
}));

const mockToast = toast as typeof toast & {
  success: ReturnType<typeof vi.fn>;
  error: ReturnType<typeof vi.fn>;
};

// Get mocked useRouter (mocked globally in vitest.setup.ts)
const mockUseRouter = vi.mocked(useRouter);

describe("useSignOut", () => {
  let queryClient: QueryClient;
  let mockLocalStorage: ReturnType<typeof setupMockLocalStorage>;
  let mockPush: ReturnType<typeof vi.fn>;
  let mockReplace: ReturnType<typeof vi.fn>;
  let originalLocation: Location;

  function setWindowLocation(url: string) {
    const location = new URL(url);
    const mockLocation: Partial<Location> & { href: string } = {
      href: location.href,
      origin: location.origin,
      protocol: location.protocol,
      host: location.host,
      hostname: location.hostname,
      port: location.port,
      pathname: location.pathname,
      search: location.search,
      hash: location.hash,
      assign: vi.fn(),
      replace: vi.fn(),
      reload: vi.fn(),
      toString: () => location.href,
    };

    Object.defineProperty(window, "location", {
      value: mockLocation,
      writable: true,
      configurable: true,
    });
  }

  beforeEach(() => {
    vi.clearAllMocks();

    queryClient = createTestQueryClient();
    mockLocalStorage = setupMockLocalStorage();
    originalLocation = window.location;

    // Create fresh mock functions for router
    mockPush = vi.fn();
    mockReplace = vi.fn();

    // Override the global mock with our tracked functions
    mockUseRouter.mockReturnValue({
      push: mockPush,
      replace: mockReplace,
      back: vi.fn(),
      forward: vi.fn(),
      refresh: vi.fn(),
      prefetch: vi.fn(),
    } as ReturnType<typeof useRouter>);

    // Mock client.POST to simulate successful API call
    vi.spyOn(client, "POST").mockResolvedValue({
      data: undefined,
      error: undefined,
      response: new Response(null, { status: 204 }),
    } as Awaited<ReturnType<typeof client.POST>>);
  });

  afterEach(() => {
    queryClient.clear();
    mockLocalStorage.clear();
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
      configurable: true,
    });
    vi.restoreAllMocks();
  });

  it("clears localStorage on localhost sign out while preserving theme only", async () => {
    mockLocalStorage.setItem("searchaf_org_id", "org-123");
    mockLocalStorage.setItem("searchaf_project_id", "proj-456");
    mockLocalStorage.setItem("searchaf_org_slug", "my-org");
    mockLocalStorage.setItem("searchaf-theme", "system");
    mockLocalStorage.setItem("other_data", "should-be-cleared");

    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Wait for onSuccess callback to execute
    await waitFor(() => {
      // other_data should be cleared
      expect(mockLocalStorage.getItem("other_data")).toBeNull();
    });

    expect(mockLocalStorage.getItem("searchaf-theme")).toBe("system");
    expect(mockLocalStorage.getItem("searchaf_org_id")).toBeNull();
    expect(mockLocalStorage.getItem("searchaf_project_id")).toBeNull();
    expect(mockLocalStorage.getItem("searchaf_org_slug")).toBeNull();
  });

  it("preserves org and project preferences on non-localhost sign out", async () => {
    setWindowLocation("https://dashboard.searchaf.com/");

    mockLocalStorage.setItem("searchaf_org_id", "org-123");
    mockLocalStorage.setItem("searchaf_project_id", "proj-456");
    mockLocalStorage.setItem("searchaf_org_slug", "my-org");
    mockLocalStorage.setItem("searchaf-theme", "system");
    mockLocalStorage.setItem("other_data", "should-be-cleared");

    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await waitFor(() => {
      expect(mockLocalStorage.getItem("other_data")).toBeNull();
    });

    expect(mockLocalStorage.getItem("searchaf_org_id")).toBe("org-123");
    expect(mockLocalStorage.getItem("searchaf_project_id")).toBe("proj-456");
    expect(mockLocalStorage.getItem("searchaf_org_slug")).toBe("my-org");
    expect(mockLocalStorage.getItem("searchaf-theme")).toBe("system");
  });

  it("redirects to /login after sign out", async () => {
    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/login");
    });
  });

  it("shows success toast on sign out", async () => {
    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith(expect.stringMatching(/sign(ed)? out/i));
    });
  });

  it("calls API signout endpoint", async () => {
    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Verify client.POST was called with correct endpoint
    expect(client.POST).toHaveBeenCalledWith("/auth/signout", {});
  });

  it("still signs out even if API call fails", async () => {
    // Mock client.POST to reject (simulate network failure)
    vi.spyOn(client, "POST").mockRejectedValue(new Error("Network error"));

    mockLocalStorage.setItem("test_key", "test_value");

    const { result } = renderHook(() => useSignOut(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      ),
    });

    await act(async () => {
      result.current.signOut();
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should still clear localStorage and redirect even on error
    await waitFor(() => {
      expect(mockLocalStorage.getItem("test_key")).toBeNull();
    });
    expect(mockPush).toHaveBeenCalledWith("/login");
    expect(mockToast.success).toHaveBeenCalled();
  });
});
