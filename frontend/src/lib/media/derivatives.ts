/** Longest edge of the gallery thumbnail (ADR-0004). */
export const THUMB_MAX_EDGE = 400
/** Longest edge of the detail view image (ADR-0004). */
export const DETAIL_MAX_EDGE = 2048
const JPEG_QUALITY = 0.82

export interface DecodedImage {
  width: number
  height: number
  source: CanvasImageSource | string
  close?: () => void
}

/** Browser image operations, injectable so the resize logic is testable. */
export interface DerivativeCodec {
  decode(file: File): Promise<DecodedImage>
  encode(source: DecodedImage['source'], width: number, height: number): Promise<Blob>
}

export interface Derivatives {
  t: Blob
  d: Blob
}

export function fitWithin(width: number, height: number, maxEdge: number): { width: number; height: number } {
  const scale = Math.min(1, maxEdge / Math.max(width, height))
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) }
}

/**
 * Creates the thumbnail and detail JPEGs in the browser. Returns null when the
 * browser cannot decode or encode the photo (for example HEIC outside Safari);
 * the caller then uploads only the original, as ADR-0004 documents.
 */
export async function createDerivatives(file: File, codec: DerivativeCodec = browserCodec): Promise<Derivatives | null> {
  let image: DecodedImage
  try {
    image = await codec.decode(file)
  } catch {
    return null
  }
  try {
    const detailSize = fitWithin(image.width, image.height, DETAIL_MAX_EDGE)
    const d = await codec.encode(image.source, detailSize.width, detailSize.height)
    const thumbSize = fitWithin(image.width, image.height, THUMB_MAX_EDGE)
    const t = await codec.encode(image.source, thumbSize.width, thumbSize.height)
    if (t.type !== 'image/jpeg' || d.type !== 'image/jpeg') return null
    return { t, d }
  } catch {
    return null
  } finally {
    image.close?.()
  }
}

export const browserCodec: DerivativeCodec = {
  async decode(file) {
    // from-image applies the EXIF orientation so derivatives match the photo.
    const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
    return { width: bitmap.width, height: bitmap.height, source: bitmap, close: () => bitmap.close() }
  },
  async encode(source, width, height) {
    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const context = canvas.getContext('2d')
    if (context === null || typeof source === 'string') throw new Error('Canvas 2D is unavailable')
    context.imageSmoothingQuality = 'high'
    context.drawImage(source, 0, 0, width, height)
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', JPEG_QUALITY))
    // Release the backing store early; phones keep few large canvases alive.
    canvas.width = 0
    canvas.height = 0
    if (blob === null) throw new Error('JPEG encoding failed')
    return blob
  }
}
