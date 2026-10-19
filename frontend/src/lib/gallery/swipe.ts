export interface SwipeGesture {
  /** Horizontal travel in px from touch start to touch end. */
  dx: number
  /** Vertical travel in px from touch start to touch end. */
  dy: number
  /** Most fingers on the screen at once during the gesture. */
  maxTouches: number
  /** True when the page is pinch-zoomed in, so a drag pans instead. */
  zoomed: boolean
  /** True when the gesture started on a video's native controls strip. */
  onVideoControls: boolean
}

const SWIPE_DISTANCE = 50

/** Decides whether a finished touch gesture should move to another photo (S5). */
export function swipeDirection(gesture: SwipeGesture): 'previous' | 'next' | null {
  if (gesture.maxTouches > 1 || gesture.zoomed || gesture.onVideoControls) return null
  const distance = Math.abs(gesture.dx)
  if (distance <= SWIPE_DISTANCE || distance < Math.abs(gesture.dy) * 1.5) return null
  return gesture.dx < 0 ? 'next' : 'previous'
}
