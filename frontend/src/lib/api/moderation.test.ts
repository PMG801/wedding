import { describe, expect, it, vi } from 'vitest'
import { AdminSessionRequiredError } from './admin'
import { fetchHiddenPage, hasAdminSession, setMediaVisibility } from './moderation'

const id = '00000000-0000-4000-8000-000000000001'

function response(init: { status: number; body?: unknown }) {
  return {
    ok: init.status >= 200 && init.status < 300,
    status: init.status,
    json: vi.fn().mockResolvedValue(init.body)
  }
}

const hiddenRow = {
  id,
  mime: 'image/jpeg',
  confirmed_at: 1700000000,
  original_url: null,
  thumb_url: '/api/admin/media/00000000-0000-4000-8000-000000000001/preview/t',
  detail_url: null
}

describe('setMediaVisibility', () => {
  it('sends exactly the visibility body and returns the resulting state', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { id, visibility: 'hidden' } }))

    await expect(setMediaVisibility(id, 'hidden', fetchFn)).resolves.toBe('hidden')
    expect(fetchFn).toHaveBeenCalledWith(
      `/api/admin/media/${id}`,
      expect.objectContaining({
        method: 'PATCH',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ visibility: 'hidden' })
      })
    )
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(setMediaVisibility(id, 'visible', fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })

  it('throws for other failures and for responses with an unexpected visibility', async () => {
    const serverError = vi.fn().mockResolvedValue(response({ status: 500, body: { error: 'moderation_failed' } }))
    await expect(setMediaVisibility(id, 'hidden', serverError)).rejects.toThrow(Error)

    const wrongState = vi.fn().mockResolvedValue(response({ status: 200, body: { id, visibility: 'gone' } }))
    await expect(setMediaVisibility(id, 'hidden', wrongState)).rejects.toThrow(Error)
  })
})

describe('fetchHiddenPage', () => {
  it('requests the hidden listing with the default page size and no store cache', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [], next_cursor: null } }))

    await expect(fetchHiddenPage({ cursor: null, fetchFn })).resolves.toEqual({ items: [], nextCursor: null })
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/media?visibility=hidden&limit=50',
      expect.objectContaining({ credentials: 'same-origin', cache: 'no-store' })
    )
  })

  it('encodes the cursor and maps hidden rows to camelCase items without originals', async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValue(response({ status: 200, body: { items: [hiddenRow], next_cursor: 'a+b/c' } }))

    const page = await fetchHiddenPage({ cursor: 'a+b/c', fetchFn })

    expect(fetchFn.mock.calls[0][0]).toBe('/api/admin/media?visibility=hidden&limit=50&cursor=a%2Bb%2Fc')
    expect(page).toEqual({
      items: [
        {
          id,
          mime: 'image/jpeg',
          confirmedAt: 1700000000,
          originalUrl: null,
          thumbUrl: hiddenRow.thumb_url,
          detailUrl: null
        }
      ],
      nextCursor: 'a+b/c'
    })
  })

  it('rejects rows that expose an original URL or miss fields', async () => {
    const exposed = { ...hiddenRow, original_url: '/m/o/x.jpg' }
    const fetchExposed = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [exposed], next_cursor: null } }))
    await expect(fetchHiddenPage({ cursor: null, fetchFn: fetchExposed })).rejects.toThrow(Error)

    const { mime: _omitted, ...incomplete } = hiddenRow
    const fetchIncomplete = vi
      .fn()
      .mockResolvedValue(response({ status: 200, body: { items: [incomplete], next_cursor: null } }))
    await expect(fetchHiddenPage({ cursor: null, fetchFn: fetchIncomplete })).rejects.toThrow(Error)
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401 }))

    await expect(fetchHiddenPage({ cursor: null, fetchFn })).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })
})

describe('hasAdminSession', () => {
  const status = {
    uploadsEnabled: true,
    diskPaused: false,
    freeBytes: 1,
    reservedBytes: 0,
    minFreeBytes: 0
  }

  it('is true when the admin status endpoint answers 200', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: status }))

    await expect(hasAdminSession(fetchFn)).resolves.toBe(true)
  })

  it('is false without an admin session and on any other failure', async () => {
    const unauthorized = vi.fn().mockResolvedValue(response({ status: 401 }))
    await expect(hasAdminSession(unauthorized)).resolves.toBe(false)

    const broken = vi.fn().mockRejectedValue(new Error('offline'))
    await expect(hasAdminSession(broken)).resolves.toBe(false)
  })
})
