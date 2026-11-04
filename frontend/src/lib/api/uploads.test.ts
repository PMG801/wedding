import { afterEach, describe, expect, it, vi } from 'vitest'
import type { HttpResponse, Upload } from 'tus-js-client'
import { NetworkError, type OriginalUpload, type UploadResponse } from './media'
import { createTusTransport, TUS_CHUNK_SIZE, type TusUpload } from './uploads'
import { UploadQueue } from '../upload/queue'

type TusOptions = ConstructorParameters<typeof Upload>[1]

const id = '123e4567-e89b-42d3-a456-426614174000'
const file = new File([new Uint8Array(10)], 'IMG_1.HEIC', { type: 'image/heic' })
const original: OriginalUpload = { id, file }

/** Test double for tus-js-client's Upload: the test drives its callbacks by hand. */
class FakeUpload implements TusUpload {
  started = 0
  aborts: (boolean | undefined)[] = []

  constructor(
    readonly file: File,
    readonly options: TusOptions
  ) {}

  start(): void {
    this.started++
  }

  abort(shouldTerminate?: boolean): Promise<void> {
    this.aborts.push(shouldTerminate)
    return Promise.resolve()
  }
}

function setup(options: { stallTimeoutMs?: number } = {}) {
  const uploads: FakeUpload[] = []
  const transport = createTusTransport({
    createUpload: (uploadFile, uploadOptions) => {
      const upload = new FakeUpload(uploadFile, uploadOptions)
      uploads.push(upload)
      return upload
    },
    ...options
  })
  return { transport, uploads }
}

/** A stand-in for tus-js-client's HttpResponse. */
function tusResponse(status: number, { body = '', retryAfter }: { body?: string; retryAfter?: string } = {}): HttpResponse {
  return {
    getStatus: () => status,
    getHeader: (name: string) => (name.toLowerCase() === 'retry-after' ? retryAfter : undefined),
    getBody: () => body,
    getUnderlyingObject: () => undefined
  }
}

/** An error shaped like tus-js-client's DetailedError for a non-2xx HTTP response. */
function tusHttpError(status: number, options: { body?: string; retryAfter?: string } = {}): Error {
  return Object.assign(new Error('tus: unexpected response'), { originalRequest: {}, originalResponse: tusResponse(status, options) })
}

function track(promise: Promise<UploadResponse>) {
  const outcome: { settled: boolean; response?: UploadResponse; error?: unknown } = { settled: false }
  promise.then(
    (response) => Object.assign(outcome, { settled: true, response }),
    (error: unknown) => Object.assign(outcome, { settled: true, error })
  )
  return outcome
}

afterEach(() => {
  vi.useRealTimers()
})

describe('tus upload options', () => {
  it('creates the upload at /api/uploads/ with the media ID as its resource and metadata', () => {
    const { transport, uploads } = setup()
    void transport(original, () => {})

    expect(uploads).toHaveLength(1)
    expect(uploads[0].started).toBe(1)
    expect(uploads[0].options).toEqual(
      expect.objectContaining({
        endpoint: '/api/uploads/',
        uploadUrl: `/api/uploads/${id}`,
        metadata: { id, filetype: 'image/heic' },
        chunkSize: 16 * 1024 * 1024,
        retryDelays: null,
        storeFingerprintForResuming: false
      })
    )
  })

  it('adds the capture time as captured_at metadata only when one was found', () => {
    const { transport, uploads } = setup()
    void transport({ id, file, capturedAt: '2026-10-18T18:30:15' }, () => {})
    void transport(original, () => {})

    expect(uploads[0].options.metadata).toEqual({ id, filetype: 'image/heic', captured_at: '2026-10-18T18:30:15' })
    expect(uploads[1].options.metadata).toEqual({ id, filetype: 'image/heic' })
  })

  it('sends 16 MiB chunks', () => {
    expect(TUS_CHUNK_SIZE).toBe(16 * 1024 * 1024)
  })
})

describe('tus outcomes', () => {
  it('resolves with a success response when tus reports the upload finished', async () => {
    const { transport, uploads } = setup()
    const pending = transport(original, () => {})
    uploads[0].options.onSuccess?.({ lastResponse: tusResponse(204) })

    await expect(pending).resolves.toEqual({ status: 200, retryAfterSeconds: null, errorCode: null })
  })

  it('treats a creation 409 already_confirmed as success', async () => {
    const { transport, uploads } = setup()
    const pending = transport(original, () => {})
    uploads[0].options.onError?.(tusHttpError(409, { body: '{"error":"already_confirmed"}' }))

    await expect(pending).resolves.toEqual({ status: 200, retryAfterSeconds: null, errorCode: null })
  })

  it('keeps other 409 responses as HTTP errors', async () => {
    const { transport, uploads } = setup()
    const pending = transport(original, () => {})
    uploads[0].options.onError?.(tusHttpError(409, { body: '{"error":"upload_conflict"}' }))

    await expect(pending).resolves.toEqual({ status: 409, retryAfterSeconds: null, errorCode: 'upload_conflict' })
  })

  it.each([
    [503, '7', '{"error":"upload_capacity_reached"}', { status: 503, retryAfterSeconds: 7, errorCode: 'upload_capacity_reached' }],
    [503, undefined, '{"error":"uploads_paused"}', { status: 503, retryAfterSeconds: null, errorCode: 'uploads_paused' }],
    [413, undefined, '{"error":"too_large"}', { status: 413, retryAfterSeconds: null, errorCode: 'too_large' }],
    [415, undefined, '{"error":"unsupported_type"}', { status: 415, retryAfterSeconds: null, errorCode: 'unsupported_type' }],
    [507, undefined, '{"error":"storage_full"}', { status: 507, retryAfterSeconds: null, errorCode: 'storage_full' }],
    [401, undefined, '', { status: 401, retryAfterSeconds: null, errorCode: null }]
  ])('maps HTTP %i (Retry-After %s, body %s) into an HTTP response', async (status, retryAfter, body, expected) => {
    const { transport, uploads } = setup()
    const pending = transport(original, () => {})
    uploads[0].options.onError?.(tusHttpError(status, { body, retryAfter }))

    await expect(pending).resolves.toEqual(expected)
  })

  it('rejects with NetworkError when the request never reaches the server', async () => {
    const { transport, uploads } = setup()
    const pending = transport(original, () => {})
    uploads[0].options.onError?.(new Error('tus: failed to resume upload'))

    await expect(pending).rejects.toBeInstanceOf(NetworkError)
  })

  it('reports progress in bytes sent', () => {
    const { transport, uploads } = setup()
    const progress: number[] = []
    void transport(original, (sent) => progress.push(sent))
    uploads[0].options.onProgress?.(4, 10)
    uploads[0].options.onProgress?.(10, 10)

    expect(progress).toEqual([4, 10])
  })
})

describe('tus stall watchdog', () => {
  it('aborts the upload without terminating it on the server, then reports NetworkError', async () => {
    vi.useFakeTimers()
    const { transport, uploads } = setup({ stallTimeoutMs: 1000 })
    const outcome = track(transport(original, () => {}))

    vi.advanceTimersByTime(999)
    expect(outcome.settled).toBe(false)
    expect(uploads[0].aborts).toEqual([])
    vi.advanceTimersByTime(1)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(uploads[0].aborts).toEqual([false])
    expect(outcome.error).toBeInstanceOf(NetworkError)
  })

  it('does not abort while progress keeps arriving', async () => {
    vi.useFakeTimers()
    const { transport, uploads } = setup({ stallTimeoutMs: 1000 })
    const outcome = track(transport(original, () => {}))
    for (let sent = 1; sent <= 10; sent++) {
      vi.advanceTimersByTime(900)
      uploads[0].options.onProgress?.(sent * 100, 1000)
    }
    vi.advanceTimersByTime(900)
    expect(uploads[0].aborts).toEqual([])
    expect(outcome.settled).toBe(false)
    vi.advanceTimersByTime(100)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(outcome.error).toBeInstanceOf(NetworkError)
  })

  it('defaults to the 60 second upload stall window', async () => {
    vi.useFakeTimers()
    const { transport, uploads } = setup()
    const outcome = track(transport(original, () => {}))
    vi.advanceTimersByTime(59_999)
    expect(uploads[0].aborts).toEqual([])
    vi.advanceTimersByTime(1)
    await vi.waitFor(() => expect(outcome.settled).toBe(true))
    expect(uploads[0].aborts).toEqual([false])
  })
})

describe('queue retries with tus', () => {
  it('starts a new tus upload on the same uploadUrl after a retryable failure', async () => {
    const { transport, uploads } = setup()
    const queue = new UploadQueue({
      transport: async () => {
        throw new Error('derivatives are not expected in this test')
      },
      makeDerivatives: async () => null,
      originalTransport: transport,
      newId: () => id,
      sleep: async () => {},
      random: () => 1
    })
    queue.add([file])

    await vi.waitFor(() => expect(uploads).toHaveLength(1))
    uploads[0].options.onError?.(tusHttpError(503, { body: '{"error":"upload_capacity_reached"}', retryAfter: '0' }))
    await vi.waitFor(() => expect(uploads).toHaveLength(2))
    uploads[1].options.onSuccess?.({ lastResponse: tusResponse(204) })
    await queue.idle()

    expect(uploads.map((upload) => upload.options.uploadUrl)).toEqual([`/api/uploads/${id}`, `/api/uploads/${id}`])
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'done', errorCode: null }))
  })
})
