import { describe, expect, it, vi } from 'vitest'
import type { GalleryItem } from '../api/media'
import { detailImage, downloadFileName, downloadUrl, gridImage, hasGuestSession, parseRoute, photoHash } from './view'

const base: GalleryItem = {
  id: '123e4567-e89b-42d3-a456-426614174000',
  mime: 'image/jpeg',
  confirmedAt: Date.UTC(2027, 1, 27, 18, 30, 12),
  originalUrl: '/m/o/20270227T183012.000000000Z_123e4567-e89b-42d3-a456-426614174000.jpg',
  thumbUrl: null,
  detailUrl: null
}

describe('image selection', () => {
  it('prefers derivatives and falls back to displayable originals', () => {
    const withDerivatives = { ...base, thumbUrl: '/m/d/t.jpg', detailUrl: '/m/d/d.jpg' }
    expect(gridImage(withDerivatives)).toBe('/m/d/t.jpg')
    expect(detailImage(withDerivatives)).toBe('/m/d/d.jpg')
    expect(gridImage(base)).toBe(base.originalUrl)
    expect(detailImage({ ...base, mime: 'image/png' })).toBe(base.originalUrl)
  })

  it('never shows a HEIC original in an image element', () => {
    for (const mime of ['image/heic', 'image/heif']) {
      expect(gridImage({ ...base, mime })).toBeNull()
      expect(detailImage({ ...base, mime })).toBeNull()
      expect(detailImage({ ...base, mime, thumbUrl: '/m/d/t.jpg' })).toBe('/m/d/t.jpg')
    }
  })

  it('downloads the original as an attachment with a dated name', () => {
    expect(downloadUrl(base)).toBe(`${base.originalUrl}?dl=1`)
    expect(downloadFileName(base)).toBe('boda-2027-02-27-183012.jpg')
    expect(downloadFileName({ ...base, originalUrl: '/m/o/x.heic' })).toBe('boda-2027-02-27-183012.heic')
  })
})

describe('routes', () => {
  it('parses the home and photo hashes', () => {
    expect(parseRoute('')).toEqual({ name: 'home' })
    expect(parseRoute('#/')).toEqual({ name: 'home' })
    expect(parseRoute(photoHash(base.id))).toEqual({ name: 'photo', id: base.id })
    expect(parseRoute('#/foto/not-a-uuid')).toEqual({ name: 'home' })
    expect(parseRoute('#/otra')).toEqual({ name: 'home' })
  })
})

describe('hasGuestSession', () => {
  it('maps the session check to a boolean', async () => {
    await expect(hasGuestSession(vi.fn().mockResolvedValue({ status: 204 }))).resolves.toBe(true)
    await expect(hasGuestSession(vi.fn().mockResolvedValue({ status: 401 }))).resolves.toBe(false)
    await expect(hasGuestSession(vi.fn().mockResolvedValue({ status: 502 }))).rejects.toThrow('HTTP 502')
  })
})
