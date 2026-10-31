import { describe, expect, it, vi } from 'vitest'
import { fetchCarousels, fetchPeriodPage, PeriodNotFoundError } from './carousels'
import { GuestSessionRequiredError } from './media'

const id = '123e4567-e89b-42d3-a456-426614174000'
const item = {
  id,
  mime: 'image/jpeg',
  confirmed_at: 1790000000000,
  original_url: `/m/o/x_${id}.jpg`,
  thumb_url: `/m/d/${id}_t.jpg`,
  detail_url: null,
  like_count: 2,
  liked_by_me: false
}
const period = { id: 1, start: '2026-10-18T18:00', end: '2026-10-18T20:00', total: 14, items: [item] }

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: vi.fn().mockResolvedValue(body) }
}

describe('fetchCarousels', () => {
  it('loads the periods uncached and returns typed items and the rotation instant', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { rotates_at: 1790000600000, periods: [period] }))
    await expect(fetchCarousels({ fetchFn })).resolves.toEqual({
      rotatesAt: 1790000600000,
      periods: [
        {
          id: 1,
          start: '2026-10-18T18:00',
          end: '2026-10-18T20:00',
          total: 14,
          items: [
            {
              id,
              mime: 'image/jpeg',
              confirmedAt: 1790000000000,
              originalUrl: item.original_url,
              thumbUrl: item.thumb_url,
              detailUrl: null,
              likeCount: 2,
              likedByMe: false
            }
          ]
        }
      ]
    })
    expect(fetchFn).toHaveBeenCalledWith('/api/carousels', { credentials: 'same-origin', cache: 'no-store' })
  })

  it('returns no periods when the feature is off or no period has media', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { rotates_at: 1, periods: [] }))
    await expect(fetchCarousels({ fetchFn })).resolves.toEqual({ rotatesAt: 1, periods: [] })
  })

  it('reports a missing guest session distinctly and other failures as errors', async () => {
    await expect(fetchCarousels({ fetchFn: vi.fn().mockResolvedValue({ ok: false, status: 401 }) })).rejects.toBeInstanceOf(
      GuestSessionRequiredError
    )
    await expect(fetchCarousels({ fetchFn: vi.fn().mockResolvedValue({ ok: false, status: 500 }) })).rejects.toThrow(
      'Carousels request failed with HTTP 500'
    )
  })

  it('rejects malformed responses', async () => {
    for (const body of [
      {},
      { rotates_at: '1790000600000', periods: [] },
      { rotates_at: 1, periods: [{ ...period, id: 0 }] },
      { rotates_at: 1, periods: [{ ...period, total: -1 }] },
      { rotates_at: 1, periods: [{ ...period, items: 'x' }] },
      { rotates_at: 1, periods: [{ ...period, items: [{ ...item, like_count: '2' }] }] }
    ]) {
      await expect(fetchCarousels({ fetchFn: vi.fn().mockResolvedValue(jsonResponse(200, body)) })).rejects.toThrow(
        'Carousels response has an invalid shape'
      )
    }
  })
})

describe('fetchPeriodPage', () => {
  it('pages the album of one period with the opaque cursor', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { items: [item], next_cursor: 'n1' }))
    await expect(fetchPeriodPage({ id: 3, cursor: 'a+b', limit: 20, fetchFn })).resolves.toEqual({
      items: [expect.objectContaining({ id, likeCount: 2 })],
      nextCursor: 'n1'
    })
    expect(fetchFn).toHaveBeenCalledWith('/api/carousels/3?limit=20&cursor=a%2Bb', { credentials: 'same-origin', cache: 'no-store' })
  })

  it('asks for the first page with the default limit', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { items: [], next_cursor: null }))
    await fetchPeriodPage({ id: 1, fetchFn })
    expect(fetchFn.mock.calls[0][0]).toBe('/api/carousels/1?limit=50')
  })

  it('reports an unknown period as PeriodNotFoundError', async () => {
    await expect(fetchPeriodPage({ id: 9, fetchFn: vi.fn().mockResolvedValue(jsonResponse(404, { error: 'period_not_found' })) })).rejects.toBeInstanceOf(
      PeriodNotFoundError
    )
  })

  it('reports a missing session and rejects malformed pages', async () => {
    await expect(fetchPeriodPage({ id: 1, fetchFn: vi.fn().mockResolvedValue({ ok: false, status: 401 }) })).rejects.toBeInstanceOf(
      GuestSessionRequiredError
    )
    await expect(fetchPeriodPage({ id: 1, fetchFn: vi.fn().mockResolvedValue(jsonResponse(200, { items: [] })) })).rejects.toThrow(
      'Carousel page response has an invalid shape'
    )
  })
})
