import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createXhrTransport,
  derivativeUploadRequest,
  fetchGalleryPage,
  GuestSessionRequiredError,
  NetworkError,
  originalUploadRequest,
  type UploadRequest
} from './media'

/** Minimal XMLHttpRequest double: the test drives progress and completion by hand. */
class FakeXHR {
  static instances: FakeXHR[] = []
  upload: { onprogress: ((event: { loaded: number }) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  status = 0
  responseText = ''
  aborted = false

  constructor() {
    FakeXHR.instances.push(this)
  }
  open(): void {}
  setRequestHeader(): void {}
  getResponseHeader(): string | null {
    return null
  }
  send(): void {}
  abort(): void {
    this.aborted = true
  }
  progress(loaded: number): void {
    this.upload.onprogress?.({ loaded })
  }
}

afterEach(() => {
  FakeXHR.instances = []
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function startUpload(stallTimeoutMs?: number) {
  vi.useFakeTimers()
  vi.stubGlobal('XMLHttpRequest', FakeXHR)
  const request: UploadRequest = { method: 'PUT', url: `/api/media/${id}`, body: new Blob(['x']), contentType: 'image/jpeg' }
  const transport = createXhrTransport(stallTimeoutMs === undefined ? {} : { stallTimeoutMs })
  const outcome = { settled: false, error: null as unknown, response: null as unknown }
  transport(request, () => {}).then(
    (response) => Object.assign(outcome, { settled: true, response }),
    (error: unknown) => Object.assign(outcome, { settled: true, error })
  )
  return { xhr: FakeXHR.instances[0], outcome }
}

const id = '123e4567-e89b-42d3-a456-426614174000'

describe('upload requests', () => {
  it('sends the original as a raw PUT to its media ID', () => {
    const file = new File(['x'], 'IMG_1.HEIC', { type: 'image/heic' })
    expect(originalUploadRequest(id, file)).toEqual({
      method: 'PUT',
      url: `/api/media/${id}`,
      body: file,
      contentType: 'image/heic'
    })
  })

  it('falls back to a generic content type when the browser reports none', () => {
    const file = new File(['x'], 'IMG_1.HEIC')
    expect(originalUploadRequest(id, file).contentType).toBe('application/octet-stream')
  })

  it('posts each derivative variant as raw JPEG', () => {
    const blob = new Blob(['x'], { type: 'image/jpeg' })
    expect(derivativeUploadRequest(id, 't', blob)).toEqual({
      method: 'POST',
      url: `/api/media/${id}/derived?variant=t`,
      body: blob,
      contentType: 'image/jpeg'
    })
  })
})

describe('upload stall watchdog', () => {
  it('aborts with NetworkError once no progress event arrives for the stall window', async () => {
    const { xhr, outcome } = startUpload(1000)
    vi.advanceTimersByTime(999)
    expect(outcome.settled).toBe(false)
    vi.advanceTimersByTime(1)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(xhr.aborted).toBe(true)
    expect(outcome.error).toBeInstanceOf(NetworkError)
  })

  it('does not abort while progress keeps arriving, however long the total upload takes', async () => {
    const { xhr, outcome } = startUpload(1000)
    for (let sent = 1; sent <= 10; sent++) {
      vi.advanceTimersByTime(900)
      xhr.progress(sent * 100)
    }
    vi.advanceTimersByTime(900)
    expect(xhr.aborted).toBe(false)
    expect(outcome.settled).toBe(false)
    vi.advanceTimersByTime(100)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(outcome.error).toBeInstanceOf(NetworkError)
  })

  it('defaults to a 60 second stall window', async () => {
    const { xhr, outcome } = startUpload()
    vi.advanceTimersByTime(59_999)
    expect(xhr.aborted).toBe(false)
    vi.advanceTimersByTime(1)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(xhr.aborted).toBe(true)
  })

  it('stands down once the server responds', async () => {
    const { xhr, outcome } = startUpload(1000)
    xhr.status = 201
    xhr.onload?.()
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    vi.advanceTimersByTime(60_000)
    expect(xhr.aborted).toBe(false)
    expect(outcome.response).toEqual({ status: 201, retryAfterSeconds: null, errorCode: null })
  })
})

describe('fetchGalleryPage', () => {
  const item = {
    id,
    mime: 'image/jpeg',
    confirmed_at: 1790000000000,
    original_url: `/m/o/x_${id}.jpg`,
    thumb_url: `/m/d/${id}_t.jpg`,
    detail_url: null
  }

  it('requests the first page and returns typed items', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { items: [item], next_cursor: 'abc' }))
    await expect(fetchGalleryPage({ fetchFn })).resolves.toEqual({
      items: [
        {
          id,
          mime: 'image/jpeg',
          confirmedAt: 1790000000000,
          originalUrl: item.original_url,
          thumbUrl: item.thumb_url,
          detailUrl: null
        }
      ],
      nextCursor: 'abc'
    })
    expect(fetchFn).toHaveBeenCalledWith('/api/media?limit=50', { credentials: 'same-origin' })
  })

  it('passes the opaque cursor unchanged but URL-encoded', async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { items: [], next_cursor: null }))
    await fetchGalleryPage({ cursor: 'a+b/c', limit: 20, fetchFn })
    expect(fetchFn).toHaveBeenCalledWith('/api/media?limit=20&cursor=a%2Bb%2Fc', { credentials: 'same-origin' })
  })

  it('reports a missing guest session distinctly', async () => {
    const fetchFn = vi.fn().mockResolvedValue({ ok: false, status: 401 })
    await expect(fetchGalleryPage({ fetchFn })).rejects.toBeInstanceOf(GuestSessionRequiredError)
  })

  it('rejects other failures and malformed pages', async () => {
    await expect(fetchGalleryPage({ fetchFn: vi.fn().mockResolvedValue({ ok: false, status: 500 }) })).rejects.toThrow(
      'Gallery request failed with HTTP 500'
    )
    for (const body of [{}, { items: [{ ...item, id: 7 }], next_cursor: null }, { items: [], next_cursor: 3 }]) {
      const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, body))
      await expect(fetchGalleryPage({ fetchFn })).rejects.toThrow('Gallery response has an invalid shape')
    }
  })
})

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: vi.fn().mockResolvedValue(body) }
}
