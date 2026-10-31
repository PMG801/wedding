import { describe, expect, it } from 'vitest'
import { shouldShowBackToTop } from './backToTop'

describe('shouldShowBackToTop', () => {
  it('stays hidden before reaching about half a viewport', () => {
    expect(shouldShowBackToTop(0, 800)).toBe(false)
    expect(shouldShowBackToTop(399, 800)).toBe(false)
  })

  it('appears after scrolling about half a viewport', () => {
    expect(shouldShowBackToTop(400, 800)).toBe(true)
    expect(shouldShowBackToTop(5000, 800)).toBe(true)
  })

  it('never shows for an unknown viewport height', () => {
    expect(shouldShowBackToTop(5000, 0)).toBe(false)
  })
})
