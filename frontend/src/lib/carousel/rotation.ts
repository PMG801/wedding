/** Wait before refetching the strip after a failed request. */
export const RETRY_DELAY_MS = 60_000

const MIN_DELAY_MS = 5_000
/** The largest delay a browser timer accepts (a signed 32-bit millisecond count). */
const MAX_DELAY_MS = 2_147_483_647
/** Lets the server's rotation slot advance before the refetch, despite small clock differences. */
const SLOT_MARGIN_MS = 1_000

/** Milliseconds until the strip should refetch: the next rotation slot, never a tight loop. */
export function refreshDelayMs(rotatesAt: number, now: number): number {
  return Math.min(MAX_DELAY_MS, Math.max(MIN_DELAY_MS, rotatesAt - now + SLOT_MARGIN_MS))
}
