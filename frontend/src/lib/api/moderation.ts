import { AdminSessionRequiredError, fetchAdminStatus } from './admin'

export type MediaVisibility = 'hidden' | 'visible'

/** A hidden item as listed for the administrator; originals are never served for hidden media. */
export interface HiddenItem {
  id: string
  mime: string
  confirmedAt: number
  originalUrl: null
  thumbUrl: string | null
  detailUrl: string | null
}

export interface HiddenPage {
  items: HiddenItem[]
  nextCursor: string | null
}

const hiddenPageSize = 50

/** Sets a confirmed item's visibility (PATCH /api/admin/media/{id}) and resolves with the stored state. */
export async function setMediaVisibility(
  id: string,
  visibility: MediaVisibility,
  fetchFn: typeof fetch = fetch
): Promise<MediaVisibility> {
  const response = await fetchFn(`/api/admin/media/${id}`, {
    method: 'PATCH',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ visibility })
  })
  if (response.status === 401) throw new AdminSessionRequiredError()
  if (!response.ok) throw new Error(`Visibility request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || (body.visibility !== 'hidden' && body.visibility !== 'visible')) {
    throw new Error('Visibility response has an invalid shape')
  }
  return body.visibility
}

/** Lists one page of hidden media for the administrator (GET /api/admin/media?visibility=hidden). */
export async function fetchHiddenPage({
  cursor = null,
  fetchFn = fetch
}: { cursor?: string | null; fetchFn?: typeof fetch } = {}): Promise<HiddenPage> {
  let url = `/api/admin/media?visibility=hidden&limit=${hiddenPageSize}`
  if (cursor !== null) url += `&cursor=${encodeURIComponent(cursor)}`
  const response = await fetchFn(url, { credentials: 'same-origin', cache: 'no-store' })
  if (response.status === 401) throw new AdminSessionRequiredError()
  if (!response.ok) throw new Error(`Hidden media request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items) || !isNullableString(body.next_cursor)) {
    throw new Error('Hidden media response has an invalid shape')
  }
  return { items: body.items.map(parseHiddenItem), nextCursor: body.next_cursor }
}

/** True only when the administrator status endpoint answers, i.e. a valid admin session exists. */
export async function hasAdminSession(fetchFn: typeof fetch = fetch): Promise<boolean> {
  try {
    await fetchAdminStatus(fetchFn)
    return true
  } catch {
    return false
  }
}

function parseHiddenItem(raw: unknown): HiddenItem {
  if (
    !isRecord(raw) ||
    typeof raw.id !== 'string' ||
    typeof raw.mime !== 'string' ||
    typeof raw.confirmed_at !== 'number' ||
    raw.original_url !== null ||
    !isNullableString(raw.thumb_url) ||
    !isNullableString(raw.detail_url)
  ) {
    throw new Error('Hidden media response has an invalid shape')
  }
  return {
    id: raw.id,
    mime: raw.mime,
    confirmedAt: raw.confirmed_at,
    originalUrl: null,
    thumbUrl: raw.thumb_url,
    detailUrl: raw.detail_url
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string'
}
