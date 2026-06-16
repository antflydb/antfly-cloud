import { QueryClient } from "@tanstack/react-query";

/**
 * Create a fresh QueryClient for testing
 * Disables retries and caching to make tests faster and more predictable
 */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false, // Disable retries in tests
        gcTime: 0, // Disable caching
      },
      mutations: {
        retry: false,
      },
    },
  });
}

/**
 * Mock localStorage for testing
 * Returns an object that behaves like localStorage but is isolated per test
 */
export function setupMockLocalStorage() {
  const store: Record<string, string> = {};

  const mockLocalStorage = {
    getItem: (key: string) => store[key] || null,
    setItem: (key: string, value: string) => {
      store[key] = value;
    },
    removeItem: (key: string) => {
      delete store[key];
    },
    clear: () => {
      Object.keys(store).forEach((key) => {
        delete store[key];
      });
    },
    get length() {
      return Object.keys(store).length;
    },
    key: (index: number) => {
      const keys = Object.keys(store);
      return keys[index] || null;
    },
  };

  // Replace global localStorage
  Object.defineProperty(global, "localStorage", {
    value: mockLocalStorage,
    writable: true,
  });

  return mockLocalStorage;
}
