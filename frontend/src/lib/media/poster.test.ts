import { describe, expect, it, vi } from 'vitest'
import type { DerivativeCodec } from './derivatives'
import { createMediaDerivatives } from './poster'

function codec(width: number, height: number): DerivativeCodec {
  return {
    decode: vi.fn(async () => ({ width, height, source: 'frame' })),
    encode: vi.fn(async () => new Blob([new Uint8Array(3)], { type: 'image/jpeg' }))
  }
}

describe('createMediaDerivatives', () => {
  it('uses the video codec for videos and the photo codec for photos', async () => {
    const photo = codec(4000, 3000)
    const video = codec(3840, 2160)
    const codecs = { photo, video }

    expect(await createMediaDerivatives(new File([new Uint8Array(1)], 'a.mp4', { type: 'video/mp4' }), codecs)).not.toBeNull()
    expect(video.decode).toHaveBeenCalledTimes(1)
    expect(photo.decode).not.toHaveBeenCalled()

    await createMediaDerivatives(new File([new Uint8Array(1)], 'a.jpg', { type: 'image/jpeg' }), codecs)
    expect(photo.decode).toHaveBeenCalledTimes(1)
  })

  it('returns null so the original uploads alone when the video cannot be decoded', async () => {
    const video: DerivativeCodec = { decode: vi.fn().mockRejectedValue(new Error('HEVC unsupported')), encode: vi.fn() }
    const file = new File([new Uint8Array(1)], 'clip.mov', { type: 'video/quicktime' })
    expect(await createMediaDerivatives(file, { photo: codec(1, 1), video })).toBeNull()
  })
})
