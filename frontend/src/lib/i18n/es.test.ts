import { describe, expect, it } from 'vitest'
import { uploadErrorMessages, uploadStatusLabel } from './es'

describe('Spanish UI copy', () => {
  it('explains every upload error code to guests', () => {
    expect(Object.keys(uploadErrorMessages).sort()).toEqual(
      ['network', 'rejected', 'server', 'session_expired', 'storage_full', 'too_large', 'unsupported_type'].sort()
    )
    for (const message of Object.values(uploadErrorMessages)) expect(message.length).toBeGreaterThan(10)
    expect(uploadErrorMessages.session_expired).toContain('QR')
  })

  it('labels progress with a whole percentage', () => {
    expect(uploadStatusLabel({ status: 'uploading', progress: 0.426 })).toBe('Subiendo… 43 %')
    expect(uploadStatusLabel({ status: 'done', progress: 1 })).toBe('Subida')
    expect(uploadStatusLabel({ status: 'retrying', progress: 0.2 })).toBe('Reintentando…')
  })
})
