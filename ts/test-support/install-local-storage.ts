type StorageValue = string | null;

function createMemoryStorage(): Storage {
  const store = new Map<string, string>();

  return {
    get length() {
      return store.size;
    },
    clear() {
      store.clear();
    },
    getItem(key: string): StorageValue {
      return store.has(key) ? store.get(key)! : null;
    },
    key(index: number): StorageValue {
      return Array.from(store.keys())[index] ?? null;
    },
    removeItem(key: string) {
      store.delete(key);
    },
    setItem(key: string, value: string) {
      store.set(String(key), String(value));
    },
  };
}

/**
 * Vitest/jsdom should usually provide a working Storage implementation, but
 * some local Node/runtime combinations expose a partial stub that is missing
 * methods like clear()/getItem(). Normalize that here so frontend tests behave
 * the same locally and in CI.
 */
export function installLocalStoragePolyfill(): void {
  const current = globalThis.localStorage as Partial<Storage> | undefined;

  if (
    current &&
    typeof current.getItem === "function" &&
    typeof current.setItem === "function" &&
    typeof current.removeItem === "function" &&
    typeof current.clear === "function" &&
    typeof current.key === "function"
  ) {
    return;
  }

  Object.defineProperty(globalThis, "localStorage", {
    value: createMemoryStorage(),
    configurable: true,
    writable: true,
  });
}
