import type { UploadErrorCode, UploadItem } from './queue'

export type UploadSummaryState = 'idle' | 'uploading' | 'attention' | 'finished'

export interface UploadSummary {
  state: UploadSummaryState
  /** Valid photos, whatever their status; invalid files are counted in `rejected`. */
  total: number
  done: number
  /** Photos still queued, being prepared, uploading or retrying. */
  remaining: number
  /** Files that can never be uploaded (unsupported type or too large). */
  rejected: number
  /** Overall fraction sent, from 0 to 1, over the valid photos. */
  progress: number
  /** Failed photos that a retry can recover. */
  retryableIds: string[]
  /** Error code of the first retryable failure, to explain it once to the guest. */
  failureCode: UploadErrorCode | null
}

const PERMANENT_ERRORS = new Set<UploadErrorCode>(['unsupported_type', 'too_large'])

/** Collapses the per-photo queue into the few numbers the guest needs to see. */
export function summarizeUploads(items: readonly UploadItem[]): UploadSummary {
  let rejected = 0
  let done = 0
  let remaining = 0
  let sent = 0
  const retryableIds: string[] = []
  let failureCode: UploadErrorCode | null = null

  for (const item of items) {
    if (item.status === 'failed') {
      if (item.errorCode !== null && PERMANENT_ERRORS.has(item.errorCode)) {
        rejected++
      } else {
        retryableIds.push(item.id)
        failureCode ??= item.errorCode
      }
    } else if (item.status === 'done') {
      done++
      sent += 1
    } else {
      remaining++
      sent += Math.min(Math.max(item.progress, 0), 1)
    }
  }

  const total = items.length - rejected
  let state: UploadSummaryState
  if (items.length === 0) state = 'idle'
  else if (remaining > 0) state = 'uploading'
  else if (retryableIds.length > 0 || rejected > 0) state = 'attention'
  else state = 'finished'

  return {
    state,
    total,
    done,
    remaining,
    rejected,
    progress: total === 0 ? 0 : sent / total,
    retryableIds,
    failureCode
  }
}
