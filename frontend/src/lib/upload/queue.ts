import {
  derivativeUploadRequest,
  type OriginalTransport,
  type UploadResponse,
  type UploadTransport
} from '../api/media'
import type { Derivatives } from '../media/derivatives'
import { isVideoFile } from '../media/kind'

export type UploadStatus = 'queued' | 'preparing' | 'uploading' | 'retrying' | 'done' | 'failed'

export type UploadErrorCode =
  | 'unsupported_type'
  | 'too_large'
  | 'session_expired'
  | 'storage_full'
  | 'uploads_paused'
  | 'rejected'
  | 'network'
  | 'server'

export interface UploadItem {
  /** Client-generated UUIDv4; kept across retries so the server stays idempotent. */
  id: string
  name: string
  size: number
  status: UploadStatus
  /** Fraction of this photo's bytes sent, from 0 to 1. */
  progress: number
  errorCode: UploadErrorCode | null
  /** True when the browser could not make derivatives and only the original is sent. */
  usedFallback: boolean
}

export interface UploadQueueOptions {
  /** Sends derivatives (POST /api/media/{id}/derived). */
  transport: UploadTransport
  /** Sends originals through tus (ADR-0014). Called once per attempt; a retry resumes. */
  originalTransport: OriginalTransport
  makeDerivatives: (file: File) => Promise<Derivatives | null>
  /**
   * Reads the capture time before the original goes up (S2). Null, a failure or a stall
   * means none is sent. The default finds nothing; the app passes the real reader.
   */
  captureTime?: (file: File) => Promise<string | null>
  newId?: () => string
  sleep?: (ms: number) => Promise<void>
  onChange?: (items: readonly UploadItem[]) => void
  concurrency?: number
  maxRetries?: number
  baseDelayMs?: number
  /** Returns a number in [0, 1) for backoff jitter; injected so tests are deterministic. */
  random?: () => number
  /** Largest photo in bytes. */
  maxFileBytes?: number
  /** Largest video in bytes. */
  maxVideoBytes?: number
}

const ACCEPTED_TYPES = new Set(['image/jpeg', 'image/png', 'image/heic', 'image/heif'])
const ACCEPTED_EXTENSION = /\.(jpe?g|png|heic|heif)$/i

// 423: a tus upload still locked by an earlier attempt that the server is releasing.
const RETRYABLE_STATUSES = new Set([408, 423, 429, 500, 502, 503, 504])

/** One send of a step; called again on every retry. */
type Attempt = (onProgress: (sent: number) => void) => Promise<UploadResponse>

interface Step {
  size: number
  optional: boolean
  send: Attempt
}

/** Longest wait for the capture time before the original goes up without it. */
const CAPTURE_TIME_TIMEOUT_MS = 3_000

class UploadFailure extends Error {
  constructor(readonly code: UploadErrorCode) {
    super(code)
  }
}

/**
 * Uploads photos in selection order, two at a time (ADR-0005): derivatives
 * first, then the original, each request with bounded retries.
 */
export class UploadQueue {
  readonly #options: Required<Omit<UploadQueueOptions, 'onChange'>> & Pick<UploadQueueOptions, 'onChange'>
  readonly #items: UploadItem[] = []
  readonly #files = new Map<string, File>()
  readonly #pending: string[] = []
  #active = 0
  #videoActive = false
  #idleWaiters: (() => void)[] = []

  constructor(options: UploadQueueOptions) {
    this.#options = {
      newId: () => crypto.randomUUID(),
      captureTime: async () => null,
      sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
      concurrency: 2,
      maxRetries: 5,
      baseDelayMs: 1000,
      random: Math.random,
      maxFileBytes: 50 * 1024 * 1024,
      maxVideoBytes: 1536 * 1024 * 1024,
      ...options
    }
  }

  get items(): readonly UploadItem[] {
    return this.#items.map((item) => ({ ...item }))
  }

  add(files: Iterable<File>): void {
    for (const file of files) {
      const item: UploadItem = {
        id: this.#options.newId(),
        name: file.name,
        size: file.size,
        status: 'queued',
        progress: 0,
        errorCode: null,
        usedFallback: false
      }
      this.#items.push(item)
      const rejection = this.#validate(file)
      if (rejection !== null) {
        item.status = 'failed'
        item.errorCode = rejection
        continue
      }
      this.#files.set(item.id, file)
      this.#pending.push(item.id)
    }
    this.#emit()
    this.#pump()
  }

  /** Requeues a failed upload with the same media ID. */
  retry(id: string): void {
    const item = this.#items.find((candidate) => candidate.id === id)
    if (item === undefined || item.status !== 'failed' || !this.#files.has(id)) return
    Object.assign(item, { status: 'queued', progress: 0, errorCode: null })
    this.#pending.push(id)
    this.#emit()
    this.#pump()
  }

  /** Resolves once no photo is queued or uploading. */
  idle(): Promise<void> {
    if (this.#active === 0 && this.#pending.length === 0) return Promise.resolve()
    return new Promise((resolve) => this.#idleWaiters.push(resolve))
  }

  #validate(file: File): UploadErrorCode | null {
    const video = isVideoFile(file)
    const typeAccepted = video || (file.type === '' ? ACCEPTED_EXTENSION.test(file.name) : ACCEPTED_TYPES.has(file.type))
    if (!typeAccepted || file.size === 0) return 'unsupported_type'
    if (file.size > (video ? this.#options.maxVideoBytes : this.#options.maxFileBytes)) return 'too_large'
    return null
  }

  #pump(): void {
    // Strict FIFO. A video runs alone (ADR-0005): it waits for running uploads
    // and nothing starts behind it until it finishes.
    while (this.#active < this.#options.concurrency && this.#pending.length > 0 && !this.#videoActive) {
      const next = this.#files.get(this.#pending[0])!
      const video = isVideoFile(next)
      if (video && this.#active > 0) break
      const id = this.#pending.shift()!
      this.#videoActive = video
      this.#active++
      void this.#process(id).finally(() => {
        this.#active--
        this.#videoActive = false
        this.#pump()
        if (this.#active === 0 && this.#pending.length === 0) {
          this.#idleWaiters.splice(0).forEach((resolve) => resolve())
        }
      })
    }
  }

  async #process(id: string): Promise<void> {
    const item = this.#items.find((candidate) => candidate.id === id)!
    const file = this.#files.get(id)!
    this.#update(item, { status: 'preparing' })
    const [derivatives, capturedAt] = await Promise.all([
      this.#options.makeDerivatives(file).catch(() => null),
      captureTimeWithin(file, this.#options.captureTime, CAPTURE_TIME_TIMEOUT_MS)
    ])

    const steps: Step[] = []
    if (derivatives !== null) {
      for (const [variant, blob] of [['t', derivatives.t], ['d', derivatives.d]] as const) {
        const request = derivativeUploadRequest(id, variant, blob)
        steps.push({
          size: blob.size,
          optional: true,
          send: (onProgress) => this.#options.transport(request, onProgress)
        })
      }
    }
    steps.push({
      size: file.size,
      optional: false,
      send: (onProgress) =>
        this.#options.originalTransport(capturedAt === null ? { id, file } : { id, file, capturedAt }, onProgress)
    })
    const totalBytes = steps.reduce((sum, step) => sum + step.size, 0)
    this.#update(item, { status: 'uploading', usedFallback: derivatives === null })

    let doneBytes = 0
    try {
      for (const step of steps) {
        try {
          await this.#send(item, step.send, (sent) => {
            const progress = (doneBytes + Math.min(sent, step.size)) / totalBytes
            if (progress > item.progress) this.#update(item, { progress })
          })
        } catch (error) {
          // A missing derivative only costs the gallery preview; the original
          // still goes up. Session and disk failures affect every request.
          const code = error instanceof UploadFailure ? error.code : 'network'
          if (!step.optional || code === 'session_expired' || code === 'storage_full' || code === 'uploads_paused') throw error
        }
        doneBytes += step.size
      }
      this.#update(item, { status: 'done', progress: 1 })
    } catch (error) {
      this.#update(item, { status: 'failed', errorCode: error instanceof UploadFailure ? error.code : 'network' })
    }
  }

  /** Runs one step with bounded retries; each attempt calls `attempt` again. */
  async #send(item: UploadItem, attempt: Attempt, onProgress: (sent: number) => void): Promise<void> {
    for (let tries = 0; ; tries++) {
      let response: UploadResponse | null = null
      try {
        response = await attempt(onProgress)
      } catch {
        // Network failure: retryable like a transient server error.
      }
      if (response !== null && (response.status === 200 || response.status === 201)) return
      // An admin pause is deliberate: retrying would only hammer a closed switch.
      // The guest retries by hand from the summary.
      if (response !== null && response.status === 503 && response.errorCode === 'uploads_paused') {
        throw new UploadFailure('uploads_paused')
      }
      if (response !== null && !RETRYABLE_STATUSES.has(response.status)) {
        throw new UploadFailure(permanentFailure(response.status))
      }
      if (tries >= this.#options.maxRetries) {
        throw new UploadFailure(response === null ? 'network' : 'server')
      }
      const delay = Math.max(equalJitter(this.#options.baseDelayMs * 2 ** tries, this.#options.random), (response?.retryAfterSeconds ?? 0) * 1000)
      this.#update(item, { status: 'retrying' })
      await this.#options.sleep(delay)
      this.#update(item, { status: 'uploading' })
    }
  }

  #update(item: UploadItem, changes: Partial<UploadItem>): void {
    Object.assign(item, changes)
    this.#emit()
  }

  #emit(): void {
    this.#options.onChange?.(this.items)
  }
}

/**
 * Reads the capture time, or null when the read fails or stalls past the timeout.
 * A photo is never held back for its metadata.
 */
async function captureTimeWithin(
  file: File,
  read: (file: File) => Promise<string | null>,
  timeoutMs: number
): Promise<string | null> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const stalled = new Promise<null>((resolve) => {
    timer = setTimeout(() => resolve(null), timeoutMs)
  })
  try {
    return await Promise.race([Promise.resolve().then(() => read(file)).catch(() => null), stalled])
  } finally {
    clearTimeout(timer)
  }
}

/** Equal jitter: half the backoff is kept, the other half is random, so retries spread out without going to zero. */
export function equalJitter(backoffMs: number, random: () => number): number {
  return backoffMs / 2 + random() * (backoffMs / 2)
}

function permanentFailure(status: number): UploadErrorCode {
  if (status === 401) return 'session_expired'
  if (status === 507) return 'storage_full'
  return 'rejected'
}
