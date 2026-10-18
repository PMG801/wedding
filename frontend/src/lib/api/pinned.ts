import { AdminSessionRequiredError } from './admin'
import { GuestSessionRequiredError } from './media'

/** Image of a pinned entry: an existing gallery medium, served only while it is visible (ADR-0013, D5). */
export interface PinnedImage {
  id: string
  thumbUrl: string
  detailUrl: string | null
}

/** A pinned entry as shown to guests (GET /api/pinned). Body is plain text (ADR-0013, D4). */
export interface PinnedItem {
  id: string
  title: string
  body: string
  image: PinnedImage | null
}

/** A pinned entry as managed by the administrator (GET /api/admin/pinned). */
export interface AdminPinnedItem extends PinnedItem {
  mediaId: string | null
  position: number
  createdAt: number
  updatedAt: number
}

/** Editable fields sent on create and update. `mediaId` null means no image. */
export interface PinnedDraft {
  title: string
  body: string
  mediaId: string | null
}

export class PinnedLimitReachedError extends Error {
  constructor() {
    super('The pinned list already has five entries')
    this.name = 'PinnedLimitReachedError'
  }
}

export class InvalidPinnedItemError extends Error {
  constructor() {
    super('The server rejected the pinned entry')
    this.name = 'InvalidPinnedItemError'
  }
}

export class PinnedNotFoundError extends Error {
  constructor() {
    super('The pinned entry does not exist')
    this.name = 'PinnedNotFoundError'
  }
}

export class InvalidPinnedOrderError extends Error {
  constructor() {
    super('The pinned order does not match the current entries')
    this.name = 'InvalidPinnedOrderError'
  }
}

const jsonHeaders = { 'Content-Type': 'application/json' }

/** Guest view of the pinned entries, in position order (GET /api/pinned). */
export async function fetchPinned({ fetchFn = fetch }: { fetchFn?: typeof fetch } = {}): Promise<PinnedItem[]> {
  const response = await fetchFn('/api/pinned', { credentials: 'same-origin', cache: 'no-store' })
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (!response.ok) throw new Error(`Pinned request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items)) {
    throw new Error('Pinned response has an invalid shape')
  }
  return body.items.map(parseGuestItem)
}

/** Administrator view of the pinned entries with management fields (GET /api/admin/pinned). */
export async function fetchAdminPinned({ fetchFn = fetch }: { fetchFn?: typeof fetch } = {}): Promise<AdminPinnedItem[]> {
  const response = await fetchFn('/api/admin/pinned', { credentials: 'same-origin', cache: 'no-store' })
  if (response.status === 401) throw new AdminSessionRequiredError()
  if (!response.ok) throw new Error(`Admin pinned request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items)) {
    throw new Error('Admin pinned response has an invalid shape')
  }
  return body.items.map(parseAdminItem)
}

/** Creates an entry at the end of the order (POST /api/admin/pinned). */
export async function createPinned(draft: PinnedDraft, fetchFn: typeof fetch = fetch): Promise<AdminPinnedItem> {
  const response = await fetchFn('/api/admin/pinned', {
    method: 'POST',
    credentials: 'same-origin',
    headers: jsonHeaders,
    body: draftBody(draft)
  })
  if (!response.ok) throw await writeFailure(response)
  return parseAdminItem(await readJson(response, 'Create pinned response has an invalid shape'))
}

/** Replaces the fields of an entry; its position does not change (PUT /api/admin/pinned/{id}). */
export async function updatePinned(
  id: string,
  draft: PinnedDraft,
  fetchFn: typeof fetch = fetch
): Promise<AdminPinnedItem> {
  const response = await fetchFn(`/api/admin/pinned/${id}`, {
    method: 'PUT',
    credentials: 'same-origin',
    headers: jsonHeaders,
    body: draftBody(draft)
  })
  if (!response.ok) throw await writeFailure(response)
  return parseAdminItem(await readJson(response, 'Update pinned response has an invalid shape'))
}

/** Deletes an entry; the remaining positions are compacted by the server (DELETE /api/admin/pinned/{id}). */
export async function deletePinned(id: string, fetchFn: typeof fetch = fetch): Promise<void> {
  const response = await fetchFn(`/api/admin/pinned/${id}`, { method: 'DELETE', credentials: 'same-origin' })
  if (!response.ok) throw await writeFailure(response)
}

/** Sends the full list of ids in the new order and resolves with the reordered entries (PUT /api/admin/pinned/order). */
export async function reorderPinned(ids: readonly string[], fetchFn: typeof fetch = fetch): Promise<AdminPinnedItem[]> {
  const response = await fetchFn('/api/admin/pinned/order', {
    method: 'PUT',
    credentials: 'same-origin',
    headers: jsonHeaders,
    body: JSON.stringify({ ids })
  })
  if (!response.ok) throw await writeFailure(response)
  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items)) {
    throw new Error('Reorder pinned response has an invalid shape')
  }
  return body.items.map(parseAdminItem)
}

function draftBody(draft: PinnedDraft): string {
  return JSON.stringify({ title: draft.title, body: draft.body, media_id: draft.mediaId })
}

async function readJson(response: Response, invalidMessage: string): Promise<unknown> {
  const body: unknown = await response.json()
  if (!isRecord(body)) throw new Error(invalidMessage)
  return body
}

/** Maps a failed write to its typed error; the stable `error` code decides, not the HTTP text. */
async function writeFailure(response: Response): Promise<Error> {
  if (response.status === 401) return new AdminSessionRequiredError()
  const code = await errorCode(response)
  switch (code) {
    case 'invalid_pinned_item':
      return new InvalidPinnedItemError()
    case 'invalid_pinned_order':
      return new InvalidPinnedOrderError()
    case 'pinned_limit_reached':
      return new PinnedLimitReachedError()
    case 'pinned_not_found':
      return new PinnedNotFoundError()
    default:
      return new Error(`Pinned request failed with HTTP ${response.status}`)
  }
}

async function errorCode(response: Response): Promise<string | null> {
  try {
    const body: unknown = await response.json()
    if (isRecord(body) && typeof body.error === 'string') return body.error
  } catch {
    // Non-JSON error bodies carry no code.
  }
  return null
}

function parseGuestItem(raw: unknown): PinnedItem {
  if (!isRecord(raw) || typeof raw.id !== 'string' || typeof raw.title !== 'string' || typeof raw.body !== 'string') {
    throw new Error('Pinned response has an invalid shape')
  }
  return { id: raw.id, title: raw.title, body: raw.body, image: parseImage(raw.image) }
}

function parseAdminItem(raw: unknown): AdminPinnedItem {
  if (
    !isRecord(raw) ||
    typeof raw.id !== 'string' ||
    typeof raw.title !== 'string' ||
    typeof raw.body !== 'string' ||
    !isNullableString(raw.media_id) ||
    !isNonNegativeInteger(raw.position) ||
    typeof raw.created_at !== 'number' ||
    typeof raw.updated_at !== 'number'
  ) {
    throw new Error('Admin pinned response has an invalid shape')
  }
  return {
    id: raw.id,
    title: raw.title,
    body: raw.body,
    image: parseImage(raw.image),
    mediaId: raw.media_id,
    position: raw.position,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at
  }
}

function parseImage(raw: unknown): PinnedImage | null {
  if (raw === null) return null
  if (!isRecord(raw) || typeof raw.id !== 'string' || typeof raw.thumb_url !== 'string' || !isNullableString(raw.detail_url)) {
    throw new Error('Pinned response has an invalid shape')
  }
  return { id: raw.id, thumbUrl: raw.thumb_url, detailUrl: raw.detail_url }
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string'
}
