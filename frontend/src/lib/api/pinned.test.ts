import { describe, expect, it, vi } from 'vitest'
import { AdminSessionRequiredError } from './admin'
import {
  createPinned,
  deletePinned,
  fetchAdminPinned,
  fetchPinned,
  InvalidPinnedItemError,
  InvalidPinnedOrderError,
  PinnedLimitReachedError,
  PinnedNotFoundError,
  reorderPinned,
  updatePinned
} from './pinned'
import { GuestSessionRequiredError } from './media'

const id = '0b6f1c2e-4d3a-4f8e-9a1b-2c3d4e5f6a7b'
const otherId = '5c7d8e9f-0a1b-4c2d-8e3f-4a5b6c7d8e9f'
const mediaId = '123e4567-e89b-42d3-a456-426614174000'

function response(init: { status: number; body?: unknown }) {
  return {
    ok: init.status >= 200 && init.status < 300,
    status: init.status,
    json: vi.fn().mockResolvedValue(init.body)
  }
}

const image = {
  id: mediaId,
  thumb_url: `/m/d/${mediaId}_t.jpg`,
  detail_url: `/m/d/${mediaId}_d.jpg`
}

const guestRow = { id, title: 'Horario del día', body: '13:00 ceremonia\n14:30 aperitivo', image }

const adminRow = {
  id,
  title: 'Horario del día',
  body: '13:00 ceremonia\n14:30 aperitivo',
  media_id: mediaId,
  position: 0,
  created_at: 1790000000000,
  updated_at: 1790000100000,
  image
}

const adminItem = {
  id,
  title: 'Horario del día',
  body: '13:00 ceremonia\n14:30 aperitivo',
  mediaId,
  position: 0,
  createdAt: 1790000000000,
  updatedAt: 1790000100000,
  image: { id: mediaId, thumbUrl: image.thumb_url, detailUrl: image.detail_url }
}

const draft = { title: 'Horario del día', body: '13:00 ceremonia\n14:30 aperitivo', mediaId }

describe('fetchPinned (guest)', () => {
  it('requests GET /api/pinned with the guest credentials and no store cache', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [] } }))

    await expect(fetchPinned({ fetchFn })).resolves.toEqual([])
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/pinned',
      expect.objectContaining({ credentials: 'same-origin', cache: 'no-store' })
    )
  })

  it('maps items to camelCase and keeps a null image and a null detail URL', async () => {
    const withoutImage = { ...guestRow, id: otherId, image: null }
    const nullDetail = { ...guestRow, image: { ...image, detail_url: null } }
    const fetchFn = vi
      .fn()
      .mockResolvedValue(response({ status: 200, body: { items: [nullDetail, withoutImage] } }))

    await expect(fetchPinned({ fetchFn })).resolves.toEqual([
      {
        id,
        title: guestRow.title,
        body: guestRow.body,
        image: { id: mediaId, thumbUrl: image.thumb_url, detailUrl: null }
      },
      { id: otherId, title: guestRow.title, body: guestRow.body, image: null }
    ])
  })

  it('reports a missing guest session as GuestSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401 }))

    await expect(fetchPinned({ fetchFn })).rejects.toBeInstanceOf(GuestSessionRequiredError)
  })

  it('throws for server failures', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 503, body: { error: 'pinned_unavailable' } }))

    await expect(fetchPinned({ fetchFn })).rejects.toThrow(Error)
  })

  it('rejects responses with a wrong shape', async () => {
    const missingTitle = { ...guestRow, title: undefined }
    const numericBody = { ...guestRow, body: 5 }
    const brokenImage = { ...guestRow, image: { id: mediaId, detail_url: null } }
    for (const row of [missingTitle, numericBody, brokenImage]) {
      const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [row] } }))
      await expect(fetchPinned({ fetchFn })).rejects.toThrow(Error)
    }

    const notAList = vi.fn().mockResolvedValue(response({ status: 200, body: { items: 'nope' } }))
    await expect(fetchPinned({ fetchFn: notAList })).rejects.toThrow(Error)
  })
})

describe('fetchAdminPinned', () => {
  it('requests GET /api/admin/pinned and maps the management fields', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [adminRow] } }))

    await expect(fetchAdminPinned({ fetchFn })).resolves.toEqual([adminItem])
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/pinned',
      expect.objectContaining({ credentials: 'same-origin', cache: 'no-store' })
    )
  })

  it('accepts entries without a media reference', async () => {
    const plain = { ...adminRow, media_id: null, image: null }
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [plain] } }))

    const [item] = await fetchAdminPinned({ fetchFn })
    expect(item).toMatchObject({ mediaId: null, image: null })
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(fetchAdminPinned({ fetchFn })).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })

  it('rejects a negative or fractional position', async () => {
    for (const position of [-1, 0.5]) {
      const fetchFn = vi
        .fn()
        .mockResolvedValue(response({ status: 200, body: { items: [{ ...adminRow, position }] } }))
      await expect(fetchAdminPinned({ fetchFn })).rejects.toThrow(Error)
    }
  })
})

describe('createPinned', () => {
  it('posts the exact JSON body and returns the created entry', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 201, body: adminRow }))

    await expect(createPinned(draft, fetchFn)).resolves.toEqual(adminItem)
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/pinned',
      expect.objectContaining({
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: draft.title, body: draft.body, media_id: mediaId })
      })
    )
  })

  it('sends media_id as null when the entry has no image', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 201, body: { ...adminRow, media_id: null, image: null } }))

    await createPinned({ ...draft, mediaId: null }, fetchFn)
    expect(fetchFn.mock.calls[0][1].body).toBe(
      JSON.stringify({ title: draft.title, body: draft.body, media_id: null })
    )
  })

  it('maps pinned_limit_reached to PinnedLimitReachedError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 409, body: { error: 'pinned_limit_reached' } }))

    await expect(createPinned(draft, fetchFn)).rejects.toBeInstanceOf(PinnedLimitReachedError)
  })

  it('maps invalid_pinned_item to InvalidPinnedItemError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 400, body: { error: 'invalid_pinned_item' } }))

    await expect(createPinned(draft, fetchFn)).rejects.toBeInstanceOf(InvalidPinnedItemError)
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(createPinned(draft, fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })

  it('throws a generic error for server failures', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 500, body: { error: 'pinned_failed' } }))

    const error = await createPinned(draft, fetchFn).catch((caught: unknown) => caught)
    expect(error).toBeInstanceOf(Error)
    expect(error).not.toBeInstanceOf(PinnedLimitReachedError)
    expect(error).not.toBeInstanceOf(InvalidPinnedItemError)
  })
})

describe('updatePinned', () => {
  it('puts the exact JSON body to the entry URL and returns the updated entry', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { ...adminRow, title: 'Nuevo' } }))

    await expect(updatePinned(id, { ...draft, title: 'Nuevo' }, fetchFn)).resolves.toMatchObject({
      id,
      title: 'Nuevo'
    })
    expect(fetchFn).toHaveBeenCalledWith(
      `/api/admin/pinned/${id}`,
      expect.objectContaining({
        method: 'PUT',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: 'Nuevo', body: draft.body, media_id: mediaId })
      })
    )
  })

  it('maps pinned_not_found to PinnedNotFoundError and invalid_pinned_item to InvalidPinnedItemError', async () => {
    const missing = vi.fn().mockResolvedValue(response({ status: 404, body: { error: 'pinned_not_found' } }))
    await expect(updatePinned(id, draft, missing)).rejects.toBeInstanceOf(PinnedNotFoundError)

    const invalid = vi.fn().mockResolvedValue(response({ status: 400, body: { error: 'invalid_pinned_item' } }))
    await expect(updatePinned(id, draft, invalid)).rejects.toBeInstanceOf(InvalidPinnedItemError)
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(updatePinned(id, draft, fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })
})

describe('deletePinned', () => {
  it('sends DELETE to the entry URL and resolves on 204', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 204 }))

    await expect(deletePinned(id, fetchFn)).resolves.toBeUndefined()
    expect(fetchFn).toHaveBeenCalledWith(
      `/api/admin/pinned/${id}`,
      expect.objectContaining({ method: 'DELETE', credentials: 'same-origin' })
    )
  })

  it('maps pinned_not_found to PinnedNotFoundError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 404, body: { error: 'pinned_not_found' } }))

    await expect(deletePinned(id, fetchFn)).rejects.toBeInstanceOf(PinnedNotFoundError)
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(deletePinned(id, fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })
})

describe('reorderPinned', () => {
  it('puts the ids in the new order and returns the reordered entries', async () => {
    const second = { ...adminRow, id: otherId, position: 1 }
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: { items: [second, adminRow] } }))

    const items = await reorderPinned([otherId, id], fetchFn)
    expect(items.map((item) => item.id)).toEqual([otherId, id])
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/pinned/order',
      expect.objectContaining({
        method: 'PUT',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: [otherId, id] })
      })
    )
  })

  it('maps invalid_pinned_order to InvalidPinnedOrderError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 400, body: { error: 'invalid_pinned_order' } }))

    await expect(reorderPinned([id, id], fetchFn)).rejects.toBeInstanceOf(InvalidPinnedOrderError)
  })

  it('reports a missing admin session as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(reorderPinned([id], fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })
})
