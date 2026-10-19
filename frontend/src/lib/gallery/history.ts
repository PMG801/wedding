/** The subset of `window.history` the viewer routing needs. */
export interface HistoryLike {
  readonly state: unknown
  pushState(state: unknown, unused: string, url: string): void
  replaceState(state: unknown, unused: string, url: string): void
  back(): void
}

/** Marks the history entry that the app pushed for an open viewer. */
const VIEWER_STATE = { viewer: true } as const

function isViewerEntry(state: unknown): boolean {
  return typeof state === 'object' && state !== null && (state as { viewer?: unknown }).viewer === true
}

/** Opens the viewer on its own entry, so the phone back button closes it (S4). */
export function openViewer(history: HistoryLike, hash: string): void {
  history.pushState(VIEWER_STATE, '', hash)
}

/** Browsing photos replaces the entry so the history is not flooded. */
export function moveViewer(history: HistoryLike, hash: string): void {
  history.replaceState(history.state, '', hash)
}

/**
 * Closes the viewer. An entry the app pushed is popped, leaving the gallery
 * entry underneath; any other entry is rewritten to the gallery in place.
 */
export function closeViewer(history: HistoryLike, homeUrl: string): 'back' | 'replaced' {
  if (isViewerEntry(history.state)) {
    history.back()
    return 'back'
  }
  history.replaceState(null, '', homeUrl)
  return 'replaced'
}

/** A photo opened from a link gets a gallery entry underneath it, so back closes it. */
export function adoptDeepLink(history: HistoryLike, homeUrl: string, hash: string): void {
  if (isViewerEntry(history.state)) return
  history.replaceState(null, '', homeUrl)
  history.pushState(VIEWER_STATE, '', hash)
}
