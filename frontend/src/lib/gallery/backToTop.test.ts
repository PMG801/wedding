import { describe, expect, it } from 'vitest'
import { shouldShowBackToTop } from './backToTop'

describe('shouldShowBackToTop', () => {
  it('stays hidden near the top of the page', () => {
    expect(shouldShowBackToTop(0, 800)).toBe(false)
    expect(shouldShowBackToTop(799, 800)).toBe(false)
  })

  it('appears after scrolling down one viewport', () => {
    expect(shouldShowBackToTop(800, 800)).toBe(true)
    expect(shouldShowBackToTop(5000, 800)).toBe(true)
  })

  it('never shows for an unknown viewport height', () => {
    expect(shouldShowBackToTop(5000, 0)).toBe(false)
  })
})
