import { describe, expect, it } from 'vitest'
import { summarizeUploads } from './summary'
import type { UploadErrorCode, UploadItem, UploadStatus } from './queue'

let counter = 0
function item(status: UploadStatus, progress = 0, errorCode: UploadErrorCode | null = null): UploadItem {
  counter++
  return { id: `id-${counter}`, name: `p${counter}.jpg`, size: 10, status, progress, errorCode, usedFallback: false }
}

describe('summarizeUploads', () => {
  it('is idle with no items', () => {
    const summary = summarizeUploads([])
    expect(summary).toMatchObject({ state: 'idle', total: 0, done: 0, remaining: 0, progress: 0 })
  })

  it('counts remaining photos and averages byte progress while uploading', () => {
    const summary = summarizeUploads([item('done', 1), item('uploading', 0.5), item('queued'), item('preparing')])
    expect(summary.state).toBe('uploading')
    expect(summary.total).toBe(4)
    expect(summary.done).toBe(1)
    expect(summary.remaining).toBe(3)
    expect(summary.progress).toBeCloseTo((1 + 0.5) / 4)
  })

  it('counts retrying photos as remaining', () => {
    const summary = summarizeUploads([item('retrying', 0.2), item('done', 1)])
    expect(summary.remaining).toBe(1)
    expect(summary.state).toBe('uploading')
  })

  it('reports retryable failures with their ids and first error code', () => {
    const failed = item('failed', 0, 'network')
    const other = item('failed', 0, 'server')
    const summary = summarizeUploads([item('done', 1), failed, other])
    expect(summary.state).toBe('attention')
    expect(summary.retryableIds).toEqual([failed.id, other.id])
    expect(summary.failureCode).toBe('network')
    expect(summary.remaining).toBe(0)
  })

  it('keeps invalid files out of the progress and retry list', () => {
    const summary = summarizeUploads([item('done', 1), item('failed', 0, 'unsupported_type'), item('failed', 0, 'too_large')])
    expect(summary.rejected).toBe(2)
    expect(summary.total).toBe(1)
    expect(summary.retryableIds).toEqual([])
    expect(summary.state).toBe('attention')
  })

  it('is finished when everything valid is done and nothing failed', () => {
    const summary = summarizeUploads([item('done', 1), item('done', 1)])
    expect(summary).toMatchObject({ state: 'finished', done: 2, total: 2, progress: 1 })
  })

  it('stays uploading while some photos are pending even if others failed', () => {
    const summary = summarizeUploads([item('failed', 0, 'network'), item('uploading', 0.5)])
    expect(summary.state).toBe('uploading')
    expect(summary.retryableIds).toHaveLength(1)
  })
})
