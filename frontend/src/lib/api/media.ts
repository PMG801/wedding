export type DerivativeVariant = 't' | 'd'

export interface UploadRequest {
  method: 'PUT' | 'POST'
  url: string
  body: Blob
  contentType: string
}

export interface UploadResponse {
  status: number
  retryAfterSeconds: number | null
  errorCode: string | null
}

/** Sends one upload request. Resolves with any HTTP response; rejects only on network failure. */
export type UploadTransport = (request: UploadRequest, onProgress: (sentBytes: number) => void) => Promise<UploadResponse>

export interface GalleryItem {
  id: string
  mime: string
  confirmedAt: number
  originalUrl: string
  thumbUrl: string | null
  detailUrl: string | null
  /** Stored «me gusta» count of the medium. */
  likeCount: number
  /** Whether the current guest identity has already liked the medium. */
  likedByMe: boolean
}

export interface GalleryPage {
  items: GalleryItem[]
  nextCursor: string | null
}

/** Server state of one medium after a like change (PUT/DELETE /api/media/{id}/like). */
export interface LikeResult {
  id: string
  likeCount: number
  likedByMe: boolean
}

export class GuestSessionRequiredError extends Error {
  constructor() {
    super('A guest session from the wedding QR code is required')
    this.name = 'GuestSessionRequiredError'
  }
}

export class MediaNotFoundError extends Error {
  constructor() {
    super('The medium is unknown or hidden')
    this.name = 'MediaNotFoundError'
  }
}

export class NetworkError extends Error {
  constructor() {
    super('The upload request could not reach the server')
    this.name = 'NetworkError'
  }
}

export function originalUploadRequest(id: string, file: File): UploadRequest {
  return { method: 'PUT', url: `/api/media/${id}`, body: file, contentType: file.type || 'application/octet-stream' }
}

export function derivativeUploadRequest(id: string, variant: DerivativeVariant, jpeg: Blob): UploadRequest {
  return { method: 'POST', url: `/api/media/${id}/derived?variant=${variant}`, body: jpeg, contentType: 'image/jpeg' }
}

/**
 * Idle window before an upload with no progress event is aborted. The server
 * closes idle uploads after 90 s, so the client gives up first and retries.
 */
export const UPLOAD_STALL_TIMEOUT_MS = 60_000

export interface XhrTransportOptions {
  /** Aborts the request when no upload progress arrives for this long. There is no total deadline. */
  stallTimeoutMs?: number
}

/** Uploads with XMLHttpRequest because fetch cannot report upload progress. */
export function createXhrTransport({ stallTimeoutMs = UPLOAD_STALL_TIMEOUT_MS }: XhrTransportOptions = {}): UploadTransport {
  return (request, onProgress) =>
    new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      let watchdog: ReturnType<typeof setTimeout> | undefined
      const disarm = () => clearTimeout(watchdog)
      const arm = () => {
        disarm()
        watchdog = setTimeout(() => {
          xhr.abort()
          reject(new NetworkError())
        }, stallTimeoutMs)
      }

      xhr.open(request.method, request.url)
      xhr.setRequestHeader('Content-Type', request.contentType)
      xhr.upload.onprogress = (event) => {
        arm()
        onProgress(event.loaded)
      }
      xhr.onload = () => {
        disarm()
        const retryAfter = Number.parseInt(xhr.getResponseHeader('Retry-After') ?? '', 10)
        resolve({
          status: xhr.status,
          retryAfterSeconds: Number.isFinite(retryAfter) ? retryAfter : null,
          errorCode: parseErrorCode(xhr.responseText)
        })
      }
      xhr.onerror = () => {
        disarm()
        reject(new NetworkError())
      }
      arm()
      xhr.send(request.body)
    })
}

export const xhrTransport: UploadTransport = createXhrTransport()

function parseErrorCode(text: string): string | null {
  try {
    const body: unknown = JSON.parse(text)
    if (typeof body === 'object' && body !== null && 'error' in body && typeof body.error === 'string') {
      return body.error
    }
  } catch {
    // Non-JSON bodies (empty success, plain-text 401) carry no error code.
  }
  return null
}

export async function fetchGalleryPage({
  cursor = null,
  limit = 50,
  fetchFn = fetch
}: { cursor?: string | null; limit?: number; fetchFn?: typeof fetch } = {}): Promise<GalleryPage> {
  let url = `/api/media?limit=${limit}`
  if (cursor !== null) url += `&cursor=${encodeURIComponent(cursor)}`
  const response = await fetchFn(url, { credentials: 'same-origin' })
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (!response.ok) throw new Error(`Gallery request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items) || !isNullableString(body.next_cursor)) {
    throw new Error('Gallery response has an invalid shape')
  }
  return {
    items: body.items.map((raw) => parseGalleryItem(raw, 'Gallery response has an invalid shape')),
    nextCursor: body.next_cursor
  }
}

/** Loads the ranking of the most liked visible media (GET /api/media/top). It has no cursor and no further pages. */
export async function fetchTopMedia({ fetchFn = fetch }: { fetchFn?: typeof fetch } = {}): Promise<GalleryItem[]> {
  const response = await fetchFn('/api/media/top', { credentials: 'same-origin' })
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (!response.ok) throw new Error(`Top media request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Array.isArray(body.items)) {
    throw new Error('Top media response has an invalid shape')
  }
  return body.items.map((raw) => parseGalleryItem(raw, 'Top media response has an invalid shape'))
}

/** Marks (liked = true, PUT) or unmarks (liked = false, DELETE) a medium for this guest identity. */
export async function setLike(id: string, liked: boolean, fetchFn: typeof fetch = fetch): Promise<LikeResult> {
  const response = await fetchFn(`/api/media/${id}/like`, {
    method: liked ? 'PUT' : 'DELETE',
    credentials: 'same-origin'
  })
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (response.status === 404) throw new MediaNotFoundError()
  if (!response.ok) throw new Error(`Like request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (
    !isRecord(body) ||
    body.id !== id ||
    !isCount(body.like_count) ||
    typeof body.liked_by_me !== 'boolean'
  ) {
    throw new Error('Like response has an invalid shape')
  }
  return { id: body.id, likeCount: body.like_count, likedByMe: body.liked_by_me }
}

function parseGalleryItem(raw: unknown, invalidMessage: string): GalleryItem {
  if (
    !isRecord(raw) ||
    typeof raw.id !== 'string' ||
    typeof raw.mime !== 'string' ||
    typeof raw.confirmed_at !== 'number' ||
    typeof raw.original_url !== 'string' ||
    !isNullableString(raw.thumb_url) ||
    !isNullableString(raw.detail_url) ||
    !isCount(raw.like_count) ||
    typeof raw.liked_by_me !== 'boolean'
  ) {
    throw new Error(invalidMessage)
  }
  return {
    id: raw.id,
    mime: raw.mime,
    confirmedAt: raw.confirmed_at,
    originalUrl: raw.original_url,
    thumbUrl: raw.thumb_url,
    detailUrl: raw.detail_url,
    likeCount: raw.like_count,
    likedByMe: raw.liked_by_me
  }
}

function isCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === 'string'
}
