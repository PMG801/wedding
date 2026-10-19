import { describe, expect, it } from 'vitest'
import { swipeDirection, type SwipeGesture } from './swipe'

const base: SwipeGesture = { dx: 0, dy: 0, maxTouches: 1, zoomed: false, onVideoControls: false }

describe('swipeDirection (S5)', () => {
  it('a horizontal one-finger drag past the threshold navigates', () => {
    expect(swipeDirection({ ...base, dx: -80 })).toBe('next')
    expect(swipeDirection({ ...base, dx: 80 })).toBe('previous')
  })

  it('short drags stay put at the 50 px threshold', () => {
    expect(swipeDirection({ ...base, dx: 50 })).toBeNull()
    expect(swipeDirection({ ...base, dx: -50 })).toBeNull()
    expect(swipeDirection({ ...base, dx: 51 })).toBe('previous')
  })

  it('ignores a gesture that used two fingers at any point (pinch)', () => {
    expect(swipeDirection({ ...base, dx: -120, maxTouches: 2 })).toBeNull()
  })

  it('ignores mostly vertical drags', () => {
    expect(swipeDirection({ ...base, dx: -80, dy: 70 })).toBeNull()
    expect(swipeDirection({ ...base, dx: -80, dy: 40 })).toBe('next')
  })

  it('ignores drags while the page is zoomed in and drags on the video controls', () => {
    expect(swipeDirection({ ...base, dx: -80, zoomed: true })).toBeNull()
    expect(swipeDirection({ ...base, dx: -80, onVideoControls: true })).toBeNull()
  })
})
