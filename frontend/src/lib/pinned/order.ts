/** Maximum number of pinned entries (ADR-0013, D3). */
export const PINNED_LIMIT = 5

export function canAddPinned(count: number): boolean {
  return count < PINNED_LIMIT
}

/** Ids with the entry at `index` swapped with its neighbour `delta` places away; null when that is past an end. */
export function moveId(ids: readonly string[], index: number, delta: -1 | 1): string[] | null {
  const target = index + delta
  if (index < 0 || index >= ids.length || target < 0 || target >= ids.length) return null
  const next = [...ids]
  const moved = next[index]
  next[index] = next[target]
  next[target] = moved
  return next
}
