const VIDEO_EXTENSION = /\.(mp4|m4v|mov)$/i
const VIDEO_TYPES = new Set(['video/mp4', 'video/quicktime', 'video/x-m4v'])

/** True for MP4/M4V/MOV videos; falls back to the extension when the browser reports no type. */
export function isVideoFile(file: { name: string; type: string }): boolean {
  return file.type === '' ? VIDEO_EXTENSION.test(file.name) : VIDEO_TYPES.has(file.type)
}

/** True for a gallery MIME type that is a video. */
export function isVideoMime(mime: string): boolean {
  return mime.startsWith('video/')
}
