import type { GalleryItem } from '../api/media'
import { isVideoMime } from '../media/kind'

/** MIME types every browser can paint in an <img>; HEIC/HEIF are not among them. */
const BROWSER_DISPLAYABLE = new Set(['image/jpeg', 'image/png'])
const MEDIA_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

/** Grid tiles only ever show thumbnails; a missing one is a placeholder, never the original. */
export function gridImage(item: GalleryItem): string | null {
  return item.thumbUrl
}

export function detailImage(item: GalleryItem): string | null {
  return item.detailUrl ?? (BROWSER_DISPLAYABLE.has(item.mime) ? item.originalUrl : item.thumbUrl)
}

export function isVideoItem(item: GalleryItem): boolean {
  return isVideoMime(item.mime)
}

/** Poster shown before a video plays; undefined lets the browser show its own first frame. */
export function videoPoster(item: GalleryItem): string | undefined {
  return item.detailUrl ?? item.thumbUrl ?? undefined
}

export function downloadUrl(item: GalleryItem): string {
  return `${item.originalUrl}?dl=1`
}

export function downloadFileName(item: GalleryItem): string {
  const stamp = new Date(item.confirmedAt).toISOString().slice(0, 19).replace('T', '-').replace(/:/g, '')
  const extension = item.originalUrl.slice(item.originalUrl.lastIndexOf('.'))
  return `boda-${stamp}${extension}`
}

export type Route = { name: 'home' } | { name: 'photo'; id: string }

export function parseRoute(hash: string): Route {
  const match = /^#\/foto\/([^/]+)$/.exec(hash)
  if (match !== null && MEDIA_ID.test(match[1])) return { name: 'photo', id: match[1] }
  return { name: 'home' }
}

export function photoHash(id: string): string {
  return `#/foto/${id}`
}

/** True when the browser holds the guest cookie from the wedding QR code. */
export async function hasGuestSession(fetchFn: typeof fetch = fetch): Promise<boolean> {
  const response = await fetchFn('/api/session', { credentials: 'same-origin' })
  if (response.status === 204) return true
  if (response.status === 401) return false
  throw new Error(`Session check failed with HTTP ${response.status}`)
}

/** The grid shows the like badge only for items that already have at least one like. */
export function shouldShowLikeBadge(item: GalleryItem): boolean {
  return item.likeCount > 0
}

export interface TapPoint {
  time: number
  x: number
  y: number
}

const DOUBLE_TAP_MS = 300
const DOUBLE_TAP_DISTANCE = 40

/** True when `current` follows `previous` closely enough in time and space to be a double tap. */
export function isDoubleTap(previous: TapPoint | null, current: TapPoint): boolean {
  if (previous === null) return false
  return (
    current.time - previous.time <= DOUBLE_TAP_MS &&
    Math.hypot(current.x - previous.x, current.y - previous.y) <= DOUBLE_TAP_DISTANCE
  )
}

/**
 * Starts a video the guest just opened. Browsers may block autoplay with sound,
 * so a blocked attempt is retried muted; the guest can unmute from the controls.
 */
export async function startPlayback(video: { muted: boolean; play(): Promise<void> }): Promise<void> {
  try {
    await video.play()
  } catch {
    video.muted = true
    try {
      await video.play()
    } catch {
      // Playback stays paused; the native controls still let the guest start it.
    }
  }
}
