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
