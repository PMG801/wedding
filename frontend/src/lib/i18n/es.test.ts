import { describe, expect, it } from 'vitest'
import { copy, uploadErrorMessages, uploadSummaryLabels } from './es'

describe('Spanish UI copy', () => {
  it('explains every upload error code to guests', () => {
    expect(Object.keys(uploadErrorMessages).sort()).toEqual(
      ['network', 'rejected', 'server', 'session_expired', 'storage_full', 'too_large', 'unsupported_type'].sort()
    )
    for (const message of Object.values(uploadErrorMessages)) expect(message.length).toBeGreaterThan(10)
    expect(uploadErrorMessages.session_expired).toContain('QR')
  })

  it('words the remaining count without file names', () => {
    expect(uploadSummaryLabels.remaining(12, 40)).toBe('Quedan 12 de 40 fotos por subir')
    expect(uploadSummaryLabels.remaining(1, 40)).toBe('Queda 1 foto por subir')
  })

  it('pluralises the finished, failed and rejected messages', () => {
    expect(uploadSummaryLabels.finished(1)).toBe('✓ 1 foto subida')
    expect(uploadSummaryLabels.finished(25)).toBe('✓ 25 fotos subidas')
    expect(uploadSummaryLabels.failed(1)).toBe('Una foto no se ha podido subir')
    expect(uploadSummaryLabels.failed(3)).toBe('3 fotos no se han podido subir')
    expect(uploadSummaryLabels.rejected(1)).toContain('Un archivo no se puede subir')
    expect(uploadSummaryLabels.rejected(2)).toContain('2 archivos no se pueden subir')
    expect(uploadSummaryLabels.rejected(2)).toContain('JPEG, PNG o HEIC')
  })

  it('tells guests that hiding is reversible and deletes nothing', () => {
    expect(copy.detail.hideConfirm).toContain('No se borra')
    expect(copy.detail.hideConfirm).toContain('administración')
    expect(copy.detail.hideFailed.length).toBeGreaterThan(10)
  })

  it('offers the Recientes and Más gustadas views with an empty ranking message', () => {
    expect(copy.gallery.views).toEqual({ recent: 'Recientes', top: 'Más gustadas' })
    expect(copy.ranking.empty).toBe('Todavía no hay fotos con me gusta')
  })

  it('labels the bottom navigation upload slot with a short word', () => {
    expect(copy.upload.navLabel).toBe('Subir')
    expect(copy.upload.button).toBe('Subir fotos y vídeos')
  })

  it('labels the like button in Spanish and explains a failed like', () => {
    expect(copy.likes.like).toBe('Me gusta')
    expect(copy.likes.unlike).toBe('Quitar me gusta')
    expect(copy.likes.failed.length).toBeGreaterThan(10)
  })
})
