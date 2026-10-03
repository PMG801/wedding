import { describe, expect, it, vi } from 'vitest'
import { fetchHealth } from './health'

describe('fetchHealth', () => {
  it('returns the health status for a successful response', async () => {
    const fetchFn = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ status: 'ok' })
    })

    await expect(fetchHealth(fetchFn)).resolves.toEqual({ status: 'ok' })
    expect(fetchFn).toHaveBeenCalledWith('/api/health')
  })

  it('throws for a non-successful response', async () => {
    const fetchFn = vi.fn().mockResolvedValue({ ok: false })

    await expect(fetchHealth(fetchFn)).rejects.toThrow(Error)
  })

  it('throws when the response has an invalid shape', async () => {
    const fetchFn = vi.fn().mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ status: 42 })
    })

    await expect(fetchHealth(fetchFn)).rejects.toThrow(Error)
  })

  it('propagates network failures', async () => {
    const fetchFn = vi.fn().mockRejectedValue(new Error('network unavailable'))

    await expect(fetchHealth(fetchFn)).rejects.toThrow('network unavailable')
  })
})
