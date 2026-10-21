import { afterEach, describe, expect, it, vi } from 'vitest'
import type { DerivativeCodec } from './derivatives'
import { createMediaDerivatives, videoPosterCodec } from './poster'

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

/**
 * Minimal off-screen <video>. With `framesNeedPlayback` it behaves like Safari
 * on iOS: no frame data loads, so a seek never completes, until play() runs.
 */
class FakeVideo extends EventTarget {
  muted = false
  playsInline = false
  preload = ''
  duration = Number.NaN
  videoWidth = 0
  videoHeight = 0
  played = false
  paused = true
  #seekPending = false

  constructor(private readonly behavior: { framesNeedPlayback: boolean; playBlocked: boolean }) {
    super()
  }

  set src(_value: string) {
    queueMicrotask(() => {
      this.duration = 12.4
      this.videoWidth = 1920
      this.videoHeight = 1080
      this.dispatchEvent(new Event('loadedmetadata'))
    })
  }

  set currentTime(_seconds: number) {
    this.#seekPending = true
    this.#completeSeek()
  }

  play(): Promise<void> {
    if (this.behavior.playBlocked) return Promise.reject(new DOMException('Low Power Mode', 'NotAllowedError'))
    this.played = true
    this.paused = false
    this.#completeSeek()
    return Promise.resolve()
  }

  pause(): void {
    this.paused = true
  }

  removeAttribute(): void {}
  load(): void {}

  #completeSeek(): void {
    if (!this.#seekPending || (this.behavior.framesNeedPlayback && !this.played)) return
    this.#seekPending = false
    queueMicrotask(() => this.dispatchEvent(new Event('seeked')))
  }
}

describe('videoPosterCodec', () => {
  const clip = () => new File([new Uint8Array(1)], 'clip.mov', { type: 'video/quicktime' })

  function useVideo(behavior: { framesNeedPlayback: boolean; playBlocked: boolean }): FakeVideo {
    const video = new FakeVideo(behavior)
    const createElement = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation(((tag: string) =>
      tag === 'video' ? (video as unknown as HTMLVideoElement) : createElement(tag)) as typeof document.createElement)
    URL.createObjectURL = vi.fn(() => 'blob:clip')
    URL.revokeObjectURL = vi.fn()
    return video
  }

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('captures a frame on iOS Safari, which loads no frames until the video plays', async () => {
    vi.useFakeTimers()
    const video = useVideo({ framesNeedPlayback: true, playBlocked: false })
    const decoded = videoPosterCodec.decode(clip())
    const outcome = decoded.then(
      (image) => ({ image, error: null }),
      (error: Error) => ({ image: null, error })
    )
    await vi.advanceTimersByTimeAsync(60_000)

    const { image, error } = await outcome
    expect(error).toBeNull()
    expect(image).toMatchObject({ width: 1920, height: 1080, source: video })
    expect(video.paused).toBe(true)
  })

  it('still captures the frame when the browser refuses playback', async () => {
    useVideo({ framesNeedPlayback: false, playBlocked: true })
    await expect(videoPosterCodec.decode(clip())).resolves.toMatchObject({ width: 1920, height: 1080 })
  })

  it('gives up within 10 seconds so the original uploads without a long wait', async () => {
    vi.useFakeTimers()
    useVideo({ framesNeedPlayback: true, playBlocked: true })
    const outcome = videoPosterCodec.decode(clip()).then(
      () => 'decoded',
      (error: Error) => error.message
    )
    await vi.advanceTimersByTimeAsync(10_000)
    expect(await Promise.race([outcome, Promise.resolve('still waiting')])).toBe('Video frame timed out')
  })
})
