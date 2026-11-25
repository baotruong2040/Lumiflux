/**
 * Platform-agnostic key-value storage abstraction shared between Web and Tauri 2 (ADR-008).
 */
export interface StorageAdapter {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
  clear(): void
}

export function createMemoryStorageAdapter(): StorageAdapter {
  const store = new Map<string, string>()
  return {
    getItem(key) {
      return store.get(key) ?? null
    },
    setItem(key, value) {
      store.set(key, value)
    },
    removeItem(key) {
      store.delete(key)
    },
    clear() {
      store.clear()
    },
  }
}

export function createLocalStorageAdapter(): StorageAdapter {
  const fallback = createMemoryStorageAdapter()

  const getNativeStorage = (): Storage | null => {
    if (typeof window === 'undefined') {
      return null
    }
    try {
      return window.localStorage
    } catch {
      return null
    }
  }

  return {
    getItem(key) {
      const native = getNativeStorage()
      if (!native) return fallback.getItem(key)
      try {
        return native.getItem(key)
      } catch {
        return fallback.getItem(key)
      }
    },
    setItem(key, value) {
      const native = getNativeStorage()
      if (!native) {
        fallback.setItem(key, value)
        return
      }
      try {
        native.setItem(key, value)
      } catch {
        fallback.setItem(key, value)
      }
    },
    removeItem(key) {
      const native = getNativeStorage()
      if (!native) {
        fallback.removeItem(key)
        return
      }
      try {
        native.removeItem(key)
      } catch {
        fallback.removeItem(key)
      }
    },
    clear() {
      const native = getNativeStorage()
      if (!native) {
        fallback.clear()
        return
      }
      try {
        native.clear()
      } catch {
        fallback.clear()
      }
    },
  }
}

export function isTauriRuntime(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window
}

export const storage: StorageAdapter = createLocalStorageAdapter()
