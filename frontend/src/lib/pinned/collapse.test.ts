import { describe, expect, it, vi } from 'vitest'
import { PINNED_COLLAPSED_KEY, readCollapsed, writeCollapsed } from './collapse'

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial))
  return {
    values,
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      values.set(key, value)
    })
  }
}

describe('collapsed preference', () => {
  it('uses the boda.pinned.collapsed key', () => {
    expect(PINNED_COLLAPSED_KEY).toBe('boda.pinned.collapsed')
  })

  it('is expanded by default and only collapsed when stored as "1"', () => {
    expect(readCollapsed(memoryStorage())).toBe(false)
    expect(readCollapsed(memoryStorage({ [PINNED_COLLAPSED_KEY]: '1' }))).toBe(true)
    expect(readCollapsed(memoryStorage({ [PINNED_COLLAPSED_KEY]: '0' }))).toBe(false)
    expect(readCollapsed(memoryStorage({ [PINNED_COLLAPSED_KEY]: 'yes' }))).toBe(false)
  })

  it('falls back to expanded when storage is unavailable or throws', () => {
    expect(readCollapsed(null)).toBe(false)

    const throwing = {
      getItem: vi.fn(() => {
        throw new DOMException('denied', 'SecurityError')
      }),
      setItem: vi.fn()
    }
    expect(readCollapsed(throwing)).toBe(false)
  })

  it('writes "1" when collapsed and "0" when expanded', () => {
    const storage = memoryStorage()

    writeCollapsed(true, storage)
    expect(storage.values.get(PINNED_COLLAPSED_KEY)).toBe('1')
    writeCollapsed(false, storage)
    expect(storage.values.get(PINNED_COLLAPSED_KEY)).toBe('0')
  })

  it('never throws when writing fails', () => {
    const throwing = {
      getItem: vi.fn(),
      setItem: vi.fn(() => {
        throw new DOMException('full', 'QuotaExceededError')
      })
    }
    expect(() => writeCollapsed(true, throwing)).not.toThrow()
    expect(() => writeCollapsed(true, null)).not.toThrow()
  })
})
