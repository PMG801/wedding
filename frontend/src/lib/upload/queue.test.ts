import { describe, expect, it, vi } from 'vitest'
import type { UploadRequest, UploadResponse, UploadTransport } from '../api/media'
import type { Derivatives } from '../media/derivatives'
import { UploadQueue, type UploadItem } from './queue'

const ok201: UploadResponse = { status: 201, retryAfterSeconds: null, errorCode: null }

function photo(name: string, size = 10, type = 'image/jpeg'): File {
  return new File([new Uint8Array(size)], name, { type })
}

function sequentialIds() {
  let next = 0
  return () => `00000000-0000-4000-8000-${String(++next).padStart(12, '0')}`
}

function derivatives(): Derivatives {
  return { t: new Blob([new Uint8Array(2)], { type: 'image/jpeg' }), d: new Blob([new Uint8Array(4)], { type: 'image/jpeg' }) }
}

function setup(overrides: Partial<ConstructorParameters<typeof UploadQueue>[0]> = {}) {
  const requests: UploadRequest[] = []
  const transport: UploadTransport = vi.fn(async (request, onProgress) => {
    requests.push(request)
    onProgress(request.body.size)
    return ok201
  })
  const sleep = vi.fn(async (_ms: number) => {})
  const changes: (readonly UploadItem[])[] = []
  const queue = new UploadQueue({
    transport,
    makeDerivatives: async () => derivatives(),
    newId: sequentialIds(),
    sleep,
    onChange: (items) => changes.push(items),
    ...overrides
  })
  return { queue, requests, transport, sleep, changes }
}

describe('UploadQueue', () => {
  it('uploads the thumbnail, the detail and then the original with one media ID', async () => {
    const { queue, requests } = setup()
    queue.add([photo('a.jpg')])
    await queue.idle()

    const id = '00000000-0000-4000-8000-000000000001'
    expect(requests.map((request) => `${request.method} ${request.url}`)).toEqual([
      `POST /api/media/${id}/derived?variant=t`,
      `POST /api/media/${id}/derived?variant=d`,
      `PUT /api/media/${id}`
    ])
    expect(queue.items).toEqual([
      expect.objectContaining({ id, name: 'a.jpg', status: 'done', progress: 1, errorCode: null, usedFallback: false })
    ])
  })

  it('falls back to the original alone when the browser cannot make derivatives', async () => {
    const { queue, requests } = setup({ makeDerivatives: async () => null })
    queue.add([photo('a.heic', 10, 'image/heic')])
    await queue.idle()

    expect(requests.map((request) => request.method)).toEqual(['PUT'])
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'done', usedFallback: true }))
  })

  it('runs at most two photos at once and starts them in selection order', async () => {
    const started: string[] = []
    const releases: (() => void)[] = []
    const transport: UploadTransport = (request) => {
      started.push(request.url)
      return new Promise((resolve) => releases.push(() => resolve(ok201)))
    }
    const { queue } = setup({ transport, makeDerivatives: async () => null })
    queue.add([photo('1.jpg'), photo('2.jpg'), photo('3.jpg')])

    await vi.waitFor(() => expect(started).toHaveLength(2))
    expect(started).toEqual(['/api/media/00000000-0000-4000-8000-000000000001', '/api/media/00000000-0000-4000-8000-000000000002'])
    expect(queue.items.map((item) => item.status)).toEqual(['uploading', 'uploading', 'queued'])

    releases.shift()?.()
    await vi.waitFor(() => expect(started).toHaveLength(3))
    expect(started[2]).toBe('/api/media/00000000-0000-4000-8000-000000000003')
    releases.splice(0).forEach((release) => release())
    await queue.idle()
    expect(queue.items.map((item) => item.status)).toEqual(['done', 'done', 'done'])
  })

  it('retries transient failures with exponential backoff honoring Retry-After', async () => {
    const responses: (UploadResponse | Error)[] = [
      new Error('offline'),
      { status: 503, retryAfterSeconds: 5, errorCode: 'upload_capacity_reached' },
      { status: 408, retryAfterSeconds: null, errorCode: 'upload_idle_timeout' },
      { status: 200, retryAfterSeconds: null, errorCode: null }
    ]
    const transport: UploadTransport = vi.fn(async () => {
      const next = responses.shift()!
      if (next instanceof Error) throw next
      return next
    })
    const { queue, sleep } = setup({ transport, makeDerivatives: async () => null })
    queue.add([photo('a.jpg')])
    await queue.idle()

    expect(sleep.mock.calls.map(([ms]) => ms)).toEqual([1000, 5000, 4000])
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'done', errorCode: null }))
  })

  it('gives up after five retries and keeps the same media ID for a manual retry', async () => {
    let failing = true
    const requests: UploadRequest[] = []
    const transport: UploadTransport = vi.fn(async (request) => {
      requests.push(request)
      return failing ? { status: 500, retryAfterSeconds: null, errorCode: 'upload_failed' } : ok201
    })
    const { queue, sleep } = setup({ transport, makeDerivatives: async () => null })
    queue.add([photo('a.jpg')])
    await queue.idle()

    expect(requests).toHaveLength(6)
    expect(sleep).toHaveBeenCalledTimes(5)
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'failed', errorCode: 'server' }))

    failing = false
    queue.retry(queue.items[0].id)
    await queue.idle()
    expect(requests).toHaveLength(7)
    expect(new Set(requests.map((request) => request.url))).toEqual(new Set(['/api/media/00000000-0000-4000-8000-000000000001']))
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'done', errorCode: null, progress: 1 }))
  })

  it('stops at once on errors a retry cannot fix', async () => {
    for (const [status, errorCode] of [
      [401, 'session_expired'],
      [507, 'storage_full'],
      [415, 'rejected']
    ] as const) {
      const transport: UploadTransport = vi.fn(async () => ({ status, retryAfterSeconds: null, errorCode: null }))
      const { queue, sleep } = setup({ transport, makeDerivatives: async () => null })
      queue.add([photo('a.jpg')])
      await queue.idle()
      expect(sleep).not.toHaveBeenCalled()
      expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'failed', errorCode }))
    }
  })

  it('skips a rejected derivative but still uploads the original', async () => {
    const requests: UploadRequest[] = []
    const transport: UploadTransport = vi.fn(async (request) => {
      requests.push(request)
      return request.url.endsWith('variant=t') ? { status: 415, retryAfterSeconds: null, errorCode: null } : ok201
    })
    const { queue } = setup({ transport })
    queue.add([photo('a.jpg')])
    await queue.idle()

    expect(requests.map((request) => request.method)).toEqual(['POST', 'POST', 'PUT'])
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'done', errorCode: null }))
  })

  it('fails the whole photo when the guest session expires during a derivative', async () => {
    const transport: UploadTransport = vi.fn(async () => ({ status: 401, retryAfterSeconds: null, errorCode: null }))
    const { queue } = setup({ transport })
    queue.add([photo('a.jpg')])
    await queue.idle()
    expect(transport).toHaveBeenCalledTimes(1)
    expect(queue.items[0]).toEqual(expect.objectContaining({ status: 'failed', errorCode: 'session_expired' }))
  })

  it('rejects unsupported, empty and oversized files before uploading', async () => {
    const { queue, transport } = setup({ maxFileBytes: 100 })
    queue.add([
      photo('clip.avi', 10, 'video/x-msvideo'),
      photo('empty.jpg', 0),
      photo('huge.jpg', 101),
      photo('IMG_0001.HEIC', 10, ''),
      photo('scan.PNG', 10, '')
    ])
    await queue.idle()

    expect(queue.items.map((item) => [item.name, item.status, item.errorCode])).toEqual([
      ['clip.avi', 'failed', 'unsupported_type'],
      ['empty.jpg', 'failed', 'unsupported_type'],
      ['huge.jpg', 'failed', 'too_large'],
      ['IMG_0001.HEIC', 'done', null],
      ['scan.PNG', 'done', null]
    ])
    expect(transport).toHaveBeenCalledTimes(6)
  })

  it('reports monotonic progress across all requests of a photo', async () => {
    const transport: UploadTransport = vi.fn(async (request, onProgress) => {
      onProgress(request.body.size / 2)
      onProgress(request.body.size)
      return ok201
    })
    const { queue, changes } = setup({ transport })
    queue.add([photo('a.jpg', 10)])
    await queue.idle()

    const progress = changes.map((items) => items[0].progress)
    expect(progress).toEqual([...progress].sort((a, b) => a - b))
    expect(progress.at(-1)).toBe(1)
    expect(progress).toContain(1 / 16)
  })

  it('accepts MP4 and MOV videos and applies the video size limit only to videos', async () => {
    const { queue } = setup({ maxFileBytes: 10, maxVideoBytes: 100 })
    queue.add([photo('a.mp4', 50, 'video/mp4'), photo('b.mov', 50, ''), photo('c.jpg', 50), photo('d.mp4', 101, 'video/mp4'), photo('e.avi', 5, 'video/x-msvideo')])
    await queue.idle()
    expect(queue.items.map((item) => [item.name, item.status, item.errorCode])).toEqual([
      ['a.mp4', 'done', null],
      ['b.mov', 'done', null],
      ['c.jpg', 'failed', 'too_large'],
      ['d.mp4', 'failed', 'too_large'],
      ['e.avi', 'failed', 'unsupported_type']
    ])
  })

  it('uploads a video alone: it waits for running photos and holds back later ones', async () => {
    const running = new Set<string>()
    const log: string[] = []
    const gates: (() => void)[] = []
    const transport: UploadTransport = async (request) => {
      if (request.method !== 'PUT') return ok201
      const name = (request.body as File).name
      log.push(`start ${name} while [${[...running].join(',')}]`)
      running.add(name)
      await new Promise<void>((resolve) => gates.push(resolve))
      running.delete(name)
      return ok201
    }
    const queue = new UploadQueue({ transport, makeDerivatives: async () => null, newId: sequentialIds(), sleep: async () => {}, concurrency: 2 })
    queue.add([photo('p1.jpg'), photo('p2.jpg'), photo('v.mp4', 10, 'video/mp4'), photo('p3.jpg')])
    const release = async () => {
      await vi.waitFor(() => expect(gates.length).toBeGreaterThan(0))
      gates.shift()!()
    }
    await release()
    await release()
    await release()
    await release()
    await queue.idle()
    expect(log).toEqual(['start p1.jpg while []', 'start p2.jpg while [p1.jpg]', 'start v.mp4 while []', 'start p3.jpg while []'])
  })
})
