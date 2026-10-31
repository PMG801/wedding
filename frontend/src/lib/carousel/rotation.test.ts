import { describe, expect, it } from 'vitest'
import { refreshDelayMs, RETRY_DELAY_MS } from './rotation'

const now = Date.UTC(2026, 9, 18, 18, 10, 0)

describe('refreshDelayMs', () => {
  it('waits until the next rotation slot, plus a second so the server slot has advanced', () => {
    expect(refreshDelayMs(now + 30 * 60_000, now)).toBe(30 * 60_000 + 1_000)
  })

  it('never refetches in a tight loop when rotates_at is already past (clock skew)', () => {
    expect(refreshDelayMs(now - 60_000, now)).toBe(5_000)
    expect(refreshDelayMs(now, now)).toBe(5_000)
  })

  it('stays within the 32-bit timer limit', () => {
    expect(refreshDelayMs(now + 40 * 24 * 3600 * 1000, now)).toBe(2_147_483_647)
  })

  it('retries a failed request after a minute', () => {
    expect(RETRY_DELAY_MS).toBe(60_000)
  })
})
