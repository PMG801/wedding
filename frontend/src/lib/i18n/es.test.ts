import { describe, expect, it } from 'vitest'
import { copy, likeLabels, uploadErrorMessages, uploadSummaryLabels } from './es'

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

  it('keeps the upload strip line short and tells guests not to close the page', () => {
    expect(copy.upload.keepOpen).toBe('No cierres esta página hasta que terminen.')
    expect(copy.upload).not.toHaveProperty('hint')
  })

  it('labels the pinned toggle as an icon with words for screen readers only', () => {
    expect(copy.pinned.show).toBe('Mostrar fijados')
    expect(copy.pinned.hide).toBe('Ocultar fijados')
    expect(copy.pinned.readMore).toBe('Leer más')
    expect(copy.pinned.readLess).toBe('Leer menos')
  })

  it('announces the like action together with the photo like count', () => {
    expect(likeLabels.button(false, 0)).toBe('Me gusta, 0 me gustas')
    expect(likeLabels.button(false, 1)).toBe('Me gusta, 1 me gusta')
    expect(likeLabels.button(false, 3)).toBe('Me gusta, 3 me gustas')
    expect(likeLabels.button(true, 1)).toBe('Quitar me gusta, 1 me gusta')
    expect(likeLabels.button(true, 3)).toBe('Quitar me gusta, 3 me gustas')
  })

  it('labels the like button in Spanish and explains a failed like', () => {
    expect(copy.likes.like).toBe('Me gusta')
    expect(copy.likes.unlike).toBe('Quitar me gusta')
    expect(copy.likes.failed.length).toBeGreaterThan(10)
  })
})
