import { describe, expect, it, vi } from 'vitest'
import { createDerivatives, DETAIL_MAX_EDGE, fitWithin, THUMB_MAX_EDGE, type DerivativeCodec } from './derivatives'

describe('fitWithin', () => {
  it('scales the longest edge down to the limit and keeps the aspect ratio', () => {
    expect(fitWithin(4000, 3000, 400)).toEqual({ width: 400, height: 300 })
    expect(fitWithin(3000, 4000, 2048)).toEqual({ width: 1536, height: 2048 })
  })

  it('never upscales small images and never returns a zero edge', () => {
    expect(fitWithin(320, 200, 400)).toEqual({ width: 320, height: 200 })
    expect(fitWithin(10000, 1, 400)).toEqual({ width: 400, height: 1 })
  })
})

describe('createDerivatives', () => {
  const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' })

  it('encodes a thumbnail and a detail JPEG from one decoded image', async () => {
    const close = vi.fn()
    const codec: DerivativeCodec = {
      decode: vi.fn().mockResolvedValue({ width: 4000, height: 3000, source: 'bitmap', close }),
      encode: vi.fn(async (_source, width) => (width === 400 ? thumb : detail))
    }
    const thumb = new Blob(['t'], { type: 'image/jpeg' })
    const detail = new Blob(['d'], { type: 'image/jpeg' })

    const result = await createDerivatives(file, codec)

    expect(result).toEqual({ t: thumb, d: detail })
    expect(result?.t).toBe(thumb)
    expect(result?.d).toBe(detail)
    expect(codec.decode).toHaveBeenCalledTimes(1)
    expect(codec.encode).toHaveBeenCalledWith('bitmap', 2048, 1536)
    expect(codec.encode).toHaveBeenCalledWith('bitmap', 400, 300)
    expect(close).toHaveBeenCalledTimes(1)
    expect(THUMB_MAX_EDGE).toBe(400)
    expect(DETAIL_MAX_EDGE).toBe(2048)
  })

  it('returns null so the caller falls back to the original when decoding fails', async () => {
    const codec: DerivativeCodec = {
      decode: vi.fn().mockRejectedValue(new Error('HEIC not supported')),
      encode: vi.fn()
    }
    await expect(createDerivatives(file, codec)).resolves.toBeNull()
    expect(codec.encode).not.toHaveBeenCalled()
  })

  it('returns null and still releases the image when encoding does not yield a JPEG', async () => {
    const close = vi.fn()
    const codec: DerivativeCodec = {
      decode: vi.fn().mockResolvedValue({ width: 100, height: 100, source: 'bitmap', close }),
      encode: vi.fn().mockResolvedValue(new Blob(['x'], { type: 'image/png' }))
    }
    await expect(createDerivatives(file, codec)).resolves.toBeNull()
    expect(close).toHaveBeenCalledTimes(1)
  })
})
