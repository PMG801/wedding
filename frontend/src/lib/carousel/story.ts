export type StoryStep = 'next' | 'previous'

/** A tap or drag shorter than this counts as a tap; longer ones belong to the swipe gesture. */
const TAP_TRAVEL_LIMIT_PX = 10

/** Keeps a story index inside the items, for an empty list too. */
export function clampIndex(index: number, count: number): number {
  if (count <= 0) return 0
  return Math.min(Math.max(index, 0), count - 1)
}

/**
 * The index after one step. `previous` stops at the first item; `next` past the
 * last item reports 'end', so the caller can close the story or load more.
 */
export function storyStep(index: number, count: number, step: StoryStep): number | 'end' {
  if (count <= 0) return 'end'
  const current = clampIndex(index, count)
  if (step === 'previous') return Math.max(0, current - 1)
  return current + 1 < count ? current + 1 : 'end'
}

/** Left half of the frame goes back, right half goes on; a drag or an empty frame does nothing. */
export function storyTap(x: number, width: number, travel: number): StoryStep | null {
  if (width <= 0 || travel > TAP_TRAVEL_LIMIT_PX) return null
  return x < width / 2 ? 'previous' : 'next'
}
