import { describe, expect, it } from 'vitest'
import { bodyOverflows } from './clamp'

describe('pinned body clamp', () => {
  it('offers the read-more toggle only when the clamped text is cut off', () => {
    expect(bodyOverflows(180, 120)).toBe(true)
    expect(bodyOverflows(120, 120)).toBe(false)
  })

  it('ignores sub-pixel rounding so short bodies never show a toggle', () => {
    expect(bodyOverflows(120.4, 120)).toBe(false)
    expect(bodyOverflows(121.5, 120)).toBe(true)
  })

  it('treats a body that was never laid out as not overflowing', () => {
    expect(bodyOverflows(0, 0)).toBe(false)
  })
})
