import { describe, expect, it } from 'vitest'
import { canAddPinned, moveId, PINNED_LIMIT } from './order'

describe('canAddPinned', () => {
  it('allows creating up to five entries', () => {
    expect(PINNED_LIMIT).toBe(5)
    expect(canAddPinned(0)).toBe(true)
    expect(canAddPinned(4)).toBe(true)
    expect(canAddPinned(5)).toBe(false)
  })
})

describe('moveId', () => {
  it('swaps an entry with its neighbour in the requested direction', () => {
    expect(moveId(['a', 'b', 'c'], 0, 1)).toEqual(['b', 'a', 'c'])
    expect(moveId(['a', 'b', 'c'], 2, -1)).toEqual(['a', 'c', 'b'])
  })

  it('returns null past either end and leaves the input untouched', () => {
    const ids = ['a', 'b']
    expect(moveId(ids, 0, -1)).toBeNull()
    expect(moveId(ids, 1, 1)).toBeNull()
    expect(ids).toEqual(['a', 'b'])
  })
})
