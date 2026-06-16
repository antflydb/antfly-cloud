// Import vitest functions early
import { afterEach, vi } from "vitest";

// Import testing library
import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { installLocalStoragePolyfill } from "../../test-support/install-local-storage";

installLocalStoragePolyfill();

// Mock Next.js router
vi.mock("next/navigation", () => ({
  useRouter: vi.fn(() => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    refresh: vi.fn(),
  })),
  usePathname: () => "/",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({}),
}));

// Reset after each test
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
