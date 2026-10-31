import { describe, expect, it } from 'vitest'
import { clampIndex, storyStep, storyTap } from './story'

describe('storyStep', () => {
  it('moves to the next item and reports the end after the last one', () => {
    expect(storyStep(0, 3, 'next')).toBe(1)
    expect(storyStep(1, 3, 'next')).toBe(2)
    expect(storyStep(2, 3, 'next')).toBe('end')
  })

  it('moves back and stays on the first item', () => {
    expect(storyStep(2, 3, 'previous')).toBe(1)
    expect(storyStep(0, 3, 'previous')).toBe(0)
  })

  it('ends at once when there is nothing to show', () => {
    expect(storyStep(0, 0, 'next')).toBe('end')
    expect(storyStep(0, 0, 'previous')).toBe('end')
  })
})

describe('clampIndex', () => {
  it('keeps the index inside the items when the list shrinks or is empty', () => {
    expect(clampIndex(4, 3)).toBe(2)
    expect(clampIndex(-1, 3)).toBe(0)
    expect(clampIndex(1, 3)).toBe(1)
    expect(clampIndex(5, 0)).toBe(0)
  })
})

describe('storyTap', () => {
  it('advances on the right half and goes back on the left half', () => {
    expect(storyTap(300, 360, 2)).toBe('next')
    expect(storyTap(40, 360, 2)).toBe('previous')
  })

  it('ignores drags, which belong to the swipe gesture', () => {
    expect(storyTap(300, 360, 12)).toBeNull()
  })

  it('ignores taps on a frame with no width', () => {
    expect(storyTap(0, 0, 0)).toBeNull()
  })
})
