import {
  derivativeUploadRequest,
  originalUploadRequest,
  type UploadRequest,
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
  transport: UploadTransport
  makeDerivatives: (file: File) => Promise<Derivatives | null>
  newId?: () => string
  sleep?: (ms: number) => Promise<void>
  onChange?: (items: readonly UploadItem[]) => void
  concurrency?: number
  maxRetries?: number
  baseDelayMs?: number
  /** Largest photo in bytes. */
  maxFileBytes?: number
  /** Largest video in bytes. */
  maxVideoBytes?: number
}

const ACCEPTED_TYPES = new Set(['image/jpeg', 'image/png', 'image/heic', 'image/heif'])
const ACCEPTED_EXTENSION = /\.(jpe?g|png|heic|heif)$/i

const RETRYABLE_STATUSES = new Set([408, 429, 500, 502, 503, 504])

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
      sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
      concurrency: 2,
      maxRetries: 5,
      baseDelayMs: 1000,
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
    const derivatives = await this.#options.makeDerivatives(file).catch(() => null)

    const steps: { request: UploadRequest; optional: boolean }[] = []
    if (derivatives !== null) {
      steps.push({ request: derivativeUploadRequest(id, 't', derivatives.t), optional: true })
      steps.push({ request: derivativeUploadRequest(id, 'd', derivatives.d), optional: true })
    }
    steps.push({ request: originalUploadRequest(id, file), optional: false })
    const totalBytes = steps.reduce((sum, step) => sum + step.request.body.size, 0)
    this.#update(item, { status: 'uploading', usedFallback: derivatives === null })

    let doneBytes = 0
    try {
      for (const step of steps) {
        try {
          await this.#send(item, step.request, (sent) => {
            const progress = (doneBytes + Math.min(sent, step.request.body.size)) / totalBytes
            if (progress > item.progress) this.#update(item, { progress })
          })
        } catch (error) {
          // A missing derivative only costs the gallery preview; the original
          // still goes up. Session and disk failures affect every request.
          const code = error instanceof UploadFailure ? error.code : 'network'
          if (!step.optional || code === 'session_expired' || code === 'storage_full') throw error
        }
        doneBytes += step.request.body.size
      }
      this.#update(item, { status: 'done', progress: 1 })
    } catch (error) {
      this.#update(item, { status: 'failed', errorCode: error instanceof UploadFailure ? error.code : 'network' })
    }
  }

  async #send(item: UploadItem, request: UploadRequest, onProgress: (sent: number) => void): Promise<void> {
    for (let attempt = 0; ; attempt++) {
      let response: UploadResponse | null = null
      try {
        response = await this.#options.transport(request, onProgress)
      } catch {
        // Network failure: retryable like a transient server error.
      }
      if (response !== null && (response.status === 200 || response.status === 201)) return
      if (response !== null && !RETRYABLE_STATUSES.has(response.status)) {
        throw new UploadFailure(permanentFailure(response.status))
      }
      if (attempt >= this.#options.maxRetries) {
        throw new UploadFailure(response === null ? 'network' : 'server')
      }
      const backoff = this.#options.baseDelayMs * 2 ** attempt
      const delay = Math.max(backoff, (response?.retryAfterSeconds ?? 0) * 1000)
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

function permanentFailure(status: number): UploadErrorCode {
  if (status === 401) return 'session_expired'
  if (status === 507) return 'storage_full'
  return 'rejected'
}
