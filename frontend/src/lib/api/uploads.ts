import { Upload, type HttpResponse } from 'tus-js-client'
import {
  NetworkError,
  parseErrorCode,
  UPLOAD_STALL_TIMEOUT_MS,
  type OriginalTransport,
  type UploadResponse
} from './media'

/** tus collection of originals; each upload lives at `${TUS_ENDPOINT}${mediaId}`. */
export const TUS_ENDPOINT = '/api/uploads/'
export const TUS_CHUNK_SIZE = 16 * 1024 * 1024

type TusOptions = ConstructorParameters<typeof Upload>[1]

/** The part of tus-js-client's Upload this transport uses; tests inject a fake. */
export interface TusUpload {
  start(): void
  abort(shouldTerminate?: boolean): Promise<void>
}

export type TusUploadFactory = (file: File, options: TusOptions) => TusUpload

export interface TusTransportOptions {
  createUpload?: TusUploadFactory
  /** Aborts when no progress arrives for this long. There is no total deadline. */
  stallTimeoutMs?: number
}

const defaultCreateUpload: TusUploadFactory = (file, options) => new Upload(file, options)

/** tus has no single HTTP response for a finished upload; 200 marks it as done for the queue. */
function finished(): UploadResponse {
  return { status: 200, retryAfterSeconds: null, errorCode: null }
}

/**
 * Sends one original through tus. Each call is one attempt: tus-level retries are
 * off, so the queue owns retries and calls this again, which resumes from the server offset.
 */
export function createTusTransport({
  createUpload = defaultCreateUpload,
  stallTimeoutMs = UPLOAD_STALL_TIMEOUT_MS
}: TusTransportOptions = {}): OriginalTransport {
  return ({ id, file }, onProgress) =>
    new Promise((resolve, reject) => {
      let settled = false
      let watchdog: ReturnType<typeof setTimeout> | undefined
      let upload: TusUpload | undefined

      const settle = () => {
        settled = true
        clearTimeout(watchdog)
      }
      const arm = () => {
        clearTimeout(watchdog)
        watchdog = setTimeout(() => {
          if (settled) return
          settle()
          // Keep the server copy: the next attempt resumes from its offset.
          void upload?.abort(false).catch(() => {})
          reject(new NetworkError())
        }, stallTimeoutMs)
      }

      upload = createUpload(file, {
        endpoint: TUS_ENDPOINT,
        uploadUrl: `${TUS_ENDPOINT}${id}`,
        metadata: { id, filetype: file.type },
        chunkSize: TUS_CHUNK_SIZE,
        retryDelays: null,
        storeFingerprintForResuming: false,
        onProgress: (bytesSent) => {
          if (settled) return
          arm()
          onProgress(bytesSent)
        },
        onSuccess: () => {
          if (settled) return
          settle()
          resolve(finished())
        },
        onError: (error) => {
          if (settled) return
          settle()
          // Without an HTTP response the request never reached the server.
          const response = (error as Partial<{ originalResponse: HttpResponse | null }>).originalResponse
          if (response == null) {
            reject(new NetworkError())
            return
          }
          const status = response.getStatus()
          const errorCode = parseErrorCode(response.getBody())
          // A confirmed medium means the original is already safe: the upload succeeded.
          if (status === 409 && errorCode === 'already_confirmed') {
            resolve(finished())
            return
          }
          const retryAfter = Number.parseInt(response.getHeader('Retry-After') ?? '', 10)
          resolve({ status, retryAfterSeconds: Number.isFinite(retryAfter) ? retryAfter : null, errorCode })
        }
      })
      arm()
      upload.start()
    })
}

export const tusTransport: OriginalTransport = createTusTransport()
