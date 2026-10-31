import { describe, expect, it, vi } from 'vitest'
import type { GalleryItem } from '../api/media'
import { detailImage, isDoubleTap, shouldShowLikeBadge, downloadFileName, downloadUrl, gridImage, hasGuestSession, isVideoItem, parseRoute, photoHash, videoPoster, startPlayback, nextPreload } from './view'

const base: GalleryItem = {
  id: '123e4567-e89b-42d3-a456-426614174000',
  mime: 'image/jpeg',
  confirmedAt: Date.UTC(2027, 1, 27, 18, 30, 12),
  originalUrl: '/m/o/20270227T183012.000000000Z_123e4567-e89b-42d3-a456-426614174000.jpg',
  thumbUrl: null,
  detailUrl: null,
  likeCount: 0,
  likedByMe: false
}

describe('image selection', () => {
  it('prefers derivatives and falls back to displayable originals', () => {
    const withDerivatives = { ...base, thumbUrl: '/m/d/t.jpg', detailUrl: '/m/d/d.jpg' }
    expect(gridImage(withDerivatives)).toBe('/m/d/t.jpg')
    expect(detailImage(withDerivatives)).toBe('/m/d/d.jpg')
    expect(detailImage({ ...base, mime: 'image/png' })).toBe(base.originalUrl)
  })

  it('never shows the original in a grid tile: a missing thumbnail is a placeholder', () => {
    expect(gridImage(base)).toBeNull()
    expect(gridImage({ ...base, mime: 'image/png' })).toBeNull()
    expect(gridImage({ ...base, detailUrl: '/m/d/d.jpg' })).toBeNull()
  })

  it('treats videos as playable media and never paints the video file in an image element', () => {
    const video = { ...base, mime: 'video/mp4', originalUrl: '/m/o/clip.mp4' }
    expect(isVideoItem(video)).toBe(true)
    expect(isVideoItem(base)).toBe(false)
    expect(gridImage(video)).toBeNull()
    expect(detailImage(video)).toBeNull()
    expect(videoPoster(video)).toBeUndefined()
    const withPoster = { ...video, thumbUrl: '/m/d/t.jpg', detailUrl: '/m/d/d.jpg' }
    expect(gridImage(withPoster)).toBe('/m/d/t.jpg')
    expect(videoPoster(withPoster)).toBe('/m/d/d.jpg')
    expect(videoPoster({ ...video, thumbUrl: '/m/d/t.jpg' })).toBe('/m/d/t.jpg')
    expect(downloadFileName(video)).toMatch(/\.mp4$/)
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

describe('like affordances', () => {
  it('shows the grid like badge only once an item has a like', () => {
    expect(shouldShowLikeBadge({ ...base, likeCount: 0 })).toBe(false)
    expect(shouldShowLikeBadge({ ...base, likeCount: 1 })).toBe(true)
  })

  it('detects a double tap by time and distance', () => {
    const first = { time: 1000, x: 100, y: 100 }
    expect(isDoubleTap(null, first)).toBe(false)
    expect(isDoubleTap(first, { time: 1250, x: 110, y: 105 })).toBe(true)
    expect(isDoubleTap(first, { time: 1400, x: 100, y: 100 })).toBe(false)
    expect(isDoubleTap(first, { time: 1200, x: 200, y: 100 })).toBe(false)
  })
})

describe('startPlayback', () => {
  it('plays with sound when the browser allows it', async () => {
    const video = { muted: false, play: vi.fn().mockResolvedValue(undefined) }
    await startPlayback(video)
    expect(video.play).toHaveBeenCalledTimes(1)
    expect(video.muted).toBe(false)
  })

  it('retries muted when the browser blocks autoplay with sound', async () => {
    const play = vi.fn().mockRejectedValueOnce(new DOMException('blocked', 'NotAllowedError')).mockResolvedValue(undefined)
    const video = { muted: false, play }
    await startPlayback(video)
    expect(play).toHaveBeenCalledTimes(2)
    expect(video.muted).toBe(true)
  })

  it('gives up silently when playback is impossible even muted', async () => {
    const video = { muted: false, play: vi.fn().mockRejectedValue(new DOMException('no', 'NotAllowedError')) }
    await expect(startPlayback(video)).resolves.toBeUndefined()
  })
})

describe('nextPreload (S11)', () => {
  const photo = (id: string, extra: Partial<GalleryItem> = {}): GalleryItem => ({ ...base, id, detailUrl: `/m/d/${id}_d.jpg`, ...extra })

  it('returns only the next photo detail image', () => {
    const items = [photo('a'), photo('b'), photo('c')]
    expect(nextPreload(items, 0)).toBe('/m/d/b_d.jpg')
    expect(nextPreload(items, 1)).toBe('/m/d/c_d.jpg')
  })

  it('returns nothing at the last loaded item', () => {
    expect(nextPreload([photo('a'), photo('b')], 1)).toBeNull()
  })

  it('never preloads a video file; a video poster is preloaded instead', () => {
    const video = photo('v', { mime: 'video/mp4', originalUrl: '/m/o/v.mp4', detailUrl: null, thumbUrl: '/m/d/v_t.jpg' })
    expect(nextPreload([photo('a'), video], 0)).toBe('/m/d/v_t.jpg')
    expect(nextPreload([photo('a'), { ...video, thumbUrl: null }], 0)).toBeNull()
  })

  it('never preloads an original the browser cannot paint', () => {
    const heic = photo('h', { mime: 'image/heic', detailUrl: null, thumbUrl: null, originalUrl: '/m/o/h.heic' })
    expect(nextPreload([photo('a'), heic], 0)).toBeNull()
  })
})
