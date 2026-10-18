import { describe, expect, it } from 'vitest'
import { MAX_RENDERED_TILES, computeWindow, type WindowRequest } from './window'

const ROW = 130 // tile width + row gap
const VIEW = 800

function at(overrides: Partial<WindowRequest>) {
  return computeWindow({ scrollOffset: 0, viewportHeight: VIEW, rowHeight: ROW, columns: 3, total: 1000, ...overrides })
}

/** Rendered rows: rows touched by [start, end), counting a partial last row. */
function renderedRows(start: number, end: number, columns: number): number {
  return Math.ceil(end / columns) - start / columns
}

describe('computeWindow', () => {
  it('starts at the top with a bounded window and no top spacer', () => {
    const w = at({ scrollOffset: 0 })
    expect(MAX_RENDERED_TILES).toBe(150)
    expect(w.start).toBe(0)
    expect(w.topSpacer).toBe(0)
    expect(w.end).toBeGreaterThan(0)
    expect(w.end).toBeLessThanOrEqual(150)
    expect(w.bottomSpacer).toBeGreaterThan(0)
  })

  it('covers the viewport plus one viewport of overscan in the middle', () => {
    // Viewport shows rows 50..56 (items 150..170); overscan adds rows 43..62.
    const w = at({ scrollOffset: 6500 })
    expect(w.start).toBeLessThanOrEqual(150)
    expect(w.end).toBeGreaterThanOrEqual(171)
    expect(w.start).toBeLessThanOrEqual(129)
    expect(w.end).toBeGreaterThanOrEqual(189)
    expect(w.end - w.start).toBeLessThanOrEqual(150)
  })

  it('returns to the top window when scrolled back up', () => {
    expect(at({ scrollOffset: 6500 }).start).toBeGreaterThan(0)
    expect(at({ scrollOffset: 0 }).start).toBe(0)
  })

  it('reaches the end with no bottom spacer and never passes the total', () => {
    const w = at({ scrollOffset: 1_000_000 })
    expect(w.end).toBe(1000)
    expect(w.bottomSpacer).toBe(0)
    expect(w.end - w.start).toBeLessThanOrEqual(150)
    expect(w.start).toBeGreaterThanOrEqual(0)
  })

  it('renders everything when the total is smaller than the viewport', () => {
    expect(at({ total: 5 })).toEqual({ start: 0, end: 5, topSpacer: 0, bottomSpacer: 0 })
  })

  it('renders every item when the whole list fits under the cap (ranking of at most 50)', () => {
    expect(at({ total: 50, scrollOffset: 0 })).toEqual({ start: 0, end: 50, topSpacer: 0, bottomSpacer: 0 })
    expect(at({ total: 50, scrollOffset: 9000 })).toEqual({ start: 0, end: 50, topSpacer: 0, bottomSpacer: 0 })
    expect(at({ total: 150, scrollOffset: 9000 })).toEqual({ start: 0, end: 150, topSpacer: 0, bottomSpacer: 0 })
  })

  it('windows once the list exceeds the cap', () => {
    expect(at({ total: 151, scrollOffset: 0 }).end).toBeLessThan(151)
  })

  it('renders nothing and has no spacers for zero items', () => {
    expect(at({ total: 0, scrollOffset: 500 })).toEqual({ start: 0, end: 0, topSpacer: 0, bottomSpacer: 0 })
  })

  it('handles one column', () => {
    const w = at({ columns: 1, total: 500, scrollOffset: 10_000, rowHeight: 100, viewportHeight: 500 })
    expect(w.start).toBeGreaterThan(0)
    expect(w.end - w.start).toBeLessThanOrEqual(150)
    expect(w.end).toBeLessThanOrEqual(500)
  })

  it('handles many columns and keeps whole rows only', () => {
    for (const columns of [2, 4, 6, 10]) {
      for (const scrollOffset of [0, 1234, 6500, 40_000]) {
        const w = at({ columns, scrollOffset, total: 600 })
        expect(w.start % columns).toBe(0)
      }
    }
  })

  it('never renders more than 150 tiles (S1)', () => {
    for (const columns of [1, 2, 3, 4, 6, 8, 10]) {
      for (const viewportHeight of [200, 844, 3000, 10_000]) {
        for (const scrollOffset of [0, 3000, 25_000, 90_000]) {
          const w = at({ columns, viewportHeight, scrollOffset, total: 1000, rowHeight: 120 })
          expect(w.end - w.start).toBeLessThanOrEqual(150)
          expect(w.end - w.start).toBeGreaterThanOrEqual(0)
        }
      }
    }
  })

  it('keeps spacers plus rendered rows equal to the full grid height (S2)', () => {
    for (const total of [0, 1, 7, 150, 600, 1000]) {
      for (const columns of [1, 3, 7]) {
        for (const viewportHeight of [300, 844, 6000]) {
          for (const scrollOffset of [0, 2000, 60_000]) {
            const w = at({ total, columns, viewportHeight, scrollOffset, rowHeight: ROW })
            const fullHeight = Math.ceil(total / columns) * ROW
            const rendered = w.end > w.start ? renderedRows(w.start, w.end, columns) * ROW : 0
            expect(w.topSpacer + rendered + w.bottomSpacer).toBe(fullHeight)
          }
        }
      }
    }
  })

  it('degrades to a bounded first window when there is no layout height', () => {
    expect(at({ rowHeight: 0, total: 1000, scrollOffset: 500 })).toEqual({ start: 0, end: 150, topSpacer: 0, bottomSpacer: 0 })
    expect(at({ rowHeight: Number.NaN, total: 3 })).toEqual({ start: 0, end: 3, topSpacer: 0, bottomSpacer: 0 })
  })
})
