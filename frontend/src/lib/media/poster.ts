import { browserCodec, createDerivatives, type DecodedImage, type DerivativeCodec, type Derivatives } from './derivatives'
import { isVideoFile } from './kind'

/**
 * Longest wait for the browser to decode a video frame before the poster is
 * skipped. A working capture takes well under a second; the guest sees this
 * wait as a stalled upload when it fails.
 */
const POSTER_TIMEOUT_MS = 10_000
/** Seconds into the clip used for the poster; avoids the usual black first frame. */
const POSTER_OFFSET_SECONDS = 1

/**
 * Decodes one frame of a video in an off-screen <video> element (ADR-0004). The
 * same canvas encoder as photos then makes the thumbnail and detail JPEGs. A
 * codec the browser cannot play (for example HEVC outside Safari) rejects, and
 * the caller uploads the original alone.
 */
export const videoPosterCodec: DerivativeCodec = {
  encode: browserCodec.encode,
  decode(file) {
    const url = URL.createObjectURL(file)
    const video = document.createElement('video')
    video.muted = true
    video.playsInline = true
    video.preload = 'auto'
    const release = () => {
      video.removeAttribute('src')
      video.load()
      URL.revokeObjectURL(url)
    }
    return new Promise<DecodedImage>((resolve, reject) => {
      const fail = (reason: string) => {
        window.clearTimeout(timer)
        release()
        reject(new Error(reason))
      }
      const timer = window.setTimeout(() => fail('Video frame timed out'), POSTER_TIMEOUT_MS)
      video.addEventListener('error', () => fail('Video cannot be decoded'), { once: true })
      video.addEventListener(
        'loadedmetadata',
        () => {
          const offset = Math.min(POSTER_OFFSET_SECONDS, Math.max(0, video.duration / 10) || 0)
          void loadFrames(video).then(() => {
            video.currentTime = offset
          })
        },
        { once: true }
      )
      video.addEventListener(
        'seeked',
        () => {
          window.clearTimeout(timer)
          if (video.videoWidth === 0 || video.videoHeight === 0) return fail('Video has no picture')
          resolve({ width: video.videoWidth, height: video.videoHeight, source: video, close: release })
        },
        { once: true }
      )
      video.src = url
    })
  }
}

/**
 * Safari on iOS loads no frame data for a video that has never played, so a
 * seek on it never completes. A muted inline play() unlocks loading. When the
 * browser refuses playback (for example in Low Power Mode) the seek is still
 * tried, because other browsers load frames without it.
 */
async function loadFrames(video: HTMLVideoElement): Promise<void> {
  try {
    await video.play()
  } catch {
    // Playback refused: fall through to the seek.
  } finally {
    video.pause()
  }
}

/** Derivatives for any accepted file: a poster frame for videos, resized JPEGs for photos. */
export function createMediaDerivatives(
  file: File,
  codecs: { photo: DerivativeCodec; video: DerivativeCodec } = { photo: browserCodec, video: videoPosterCodec }
): Promise<Derivatives | null> {
  return createDerivatives(file, isVideoFile(file) ? codecs.video : codecs.photo)
}
