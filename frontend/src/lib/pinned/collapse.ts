/** localStorage key for the guest's collapsed «Fijados» section (ADR-0013, D2). Per device, not server state. */
export const PINNED_COLLAPSED_KEY = 'boda.pinned.collapsed'

export interface PreferenceStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

/** The browser's localStorage, or null when the browser denies access to it. */
function browserStorage(): PreferenceStorage | null {
  try {
    return globalThis.localStorage ?? null
  } catch {
    return null
  }
}

/** True only when the guest collapsed the section. Expanded is the default and the fallback for any failure. */
export function readCollapsed(storage: PreferenceStorage | null = browserStorage()): boolean {
  if (storage === null) return false
  try {
    return storage.getItem(PINNED_COLLAPSED_KEY) === '1'
  } catch {
    return false
  }
}

/** Remembers the choice on this device. Storage failures are ignored: the section still works for this visit. */
export function writeCollapsed(collapsed: boolean, storage: PreferenceStorage | null = browserStorage()): void {
  if (storage === null) return
  try {
    storage.setItem(PINNED_COLLAPSED_KEY, collapsed ? '1' : '0')
  } catch {
    // Quota or privacy modes reject writes; the preference is best-effort.
  }
}
