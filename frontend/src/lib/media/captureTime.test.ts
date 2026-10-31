import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  captureTimeFromHeic,
  captureTimeFromJpeg,
  captureTimeFromMp4,
  mp4CreationToLocal,
  readCaptureTime
} from './captureTime'

// ---- tiny byte fixtures -------------------------------------------------

const enc = (text: string) => new TextEncoder().encode(text)

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0))
  let offset = 0
  for (const part of parts) {
    out.set(part, offset)
    offset += part.length
  }
  return out
}

function u16(value: number, le = false): Uint8Array {
  const out = new Uint8Array(2)
  new DataView(out.buffer).setUint16(0, value, le)
  return out
}

function u32(value: number, le = false): Uint8Array {
  const out = new Uint8Array(4)
  new DataView(out.buffer).setUint32(0, value, le)
  return out
}

function u64(value: number): Uint8Array {
  const out = new Uint8Array(8)
  new DataView(out.buffer).setBigUint64(0, BigInt(value))
  return out
}

function box(type: string, payload: Uint8Array): Uint8Array {
  return concat(u32(8 + payload.length), enc(type), payload)
}

function fullBox(type: string, version: number, payload: Uint8Array): Uint8Array {
  return box(type, concat(new Uint8Array([version, 0, 0, 0]), payload))
}

/** TIFF with IFD0 → Exif IFD holding the given ASCII date tags (20 bytes each, NUL terminated). */
function tiff(le: boolean, dates: { dto?: string; dtd?: string }): Uint8Array {
  const entries: [number, string][] = []
  if (dates.dto !== undefined) entries.push([0x9003, dates.dto])
  if (dates.dtd !== undefined) entries.push([0x9004, dates.dtd])
  const exifIfd = 26
  const dataStart = exifIfd + 2 + 12 * entries.length + 4
  const out = new Uint8Array(dataStart + 20 * entries.length)
  const dv = new DataView(out.buffer)
  out.set(le ? [0x49, 0x49] : [0x4d, 0x4d], 0)
  dv.setUint16(2, 42, le)
  dv.setUint32(4, 8, le)
  // IFD0: one entry, ExifIFDPointer (0x8769, LONG) → exifIfd.
  dv.setUint16(8, 1, le)
  dv.setUint16(10, 0x8769, le)
  dv.setUint16(12, 4, le)
  dv.setUint32(14, 1, le)
  dv.setUint32(18, exifIfd, le)
  dv.setUint32(22, 0, le)
  // Exif IFD.
  dv.setUint16(exifIfd, entries.length, le)
  entries.forEach(([tag], i) => {
    const at = exifIfd + 2 + 12 * i
    dv.setUint16(at, tag, le)
    dv.setUint16(at + 2, 2, le) // ASCII
    dv.setUint32(at + 4, 20, le)
    dv.setUint32(at + 8, dataStart + 20 * i, le)
  })
  dv.setUint32(exifIfd + 2 + 12 * entries.length, 0, le)
  entries.forEach(([, text], i) => out.set(enc(`${text}\0`), dataStart + 20 * i))
  return out
}

function jpeg(exifTiff?: Uint8Array): Uint8Array {
  const app1 =
    exifTiff === undefined
      ? new Uint8Array()
      : concat(new Uint8Array([0xff, 0xe1]), u16(6 + exifTiff.length + 2), enc('Exif\0\0'), exifTiff)
  // An SOS marker ends the header; pixel data is not needed for the parser.
  return concat(new Uint8Array([0xff, 0xd8]), app1, new Uint8Array([0xff, 0xda, 0x00, 0x02, 0xff, 0xd9]))
}

/** HEIC: ftyp, meta (iinf → Exif item, iloc → offset into mdat), mdat holding the Exif item. */
function heic(exifItem: Uint8Array, { ilocVersion = 0 }: { ilocVersion?: 0 | 1 } = {}): Uint8Array {
  const ftyp = box('ftyp', concat(enc('heic'), u32(0), enc('mif1heic')))
  const infe = fullBox('infe', 2, concat(u16(1), u16(0), enc('Exif'), new Uint8Array([0])))
  const iinf = fullBox('iinf', 0, concat(u16(1), infe))
  const ilocHeader = concat(new Uint8Array([0x44, 0x00]), u16(1)) // offset/length size 4, base size 0; one item
  // The offset is a fixed-width u32, so the meta length does not depend on its value.
  const buildMeta = (offset: number) => {
    const itemEntry =
      ilocVersion === 1
        ? concat(u16(1), u16(0), u16(0), u16(1), u32(offset), u32(exifItem.length))
        : concat(u16(1), u16(0), u16(1), u32(offset), u32(exifItem.length))
    // v0: item_ID, data_ref, extent_count, extent. v1 adds construction_method (0 = file offset) after item_ID.
    const ilocBody = concat(ilocHeader, itemEntry)
    const iloc = fullBox('iloc', ilocVersion, ilocBody)
    return fullBox('meta', 0, concat(iinf, iloc))
  }
  const exifOffset = ftyp.length + buildMeta(0).length + 8
  return concat(ftyp, buildMeta(exifOffset), box('mdat', exifItem))
}

/** Exif item payload: u32 offset of the TIFF header (6 bytes of "Exif\0\0" in front), then TIFF. */
function exifItem(tiffBytes: Uint8Array): Uint8Array {
  return concat(u32(6), enc('Exif\0\0'), tiffBytes)
}

/** MP4/MOV: ftyp, then mvhd inside moov; moov placed after or before a large mdat. */
function mp4({ version, seconds, moovAtEnd = false, mdatBytes = 0 }: { version: 0 | 1; seconds: number; moovAtEnd?: boolean; mdatBytes?: number }): Uint8Array {
  const payload =
    version === 0
      ? concat(new Uint8Array([0, 0, 0, 0]), u32(seconds), new Uint8Array(20))
      : concat(new Uint8Array([1, 0, 0, 0]), u64(seconds), new Uint8Array(28))
  const moov = box('moov', box('mvhd', payload))
  const ftyp = box('ftyp', concat(enc('qt  '), u32(0), enc('qt  ')))
  const mdat = box('mdat', new Uint8Array(mdatBytes))
  return moovAtEnd ? concat(ftyp, mdat, moov) : concat(ftyp, moov, mdat)
}

/** Seconds between 1904-01-01 and 1970-01-01, the offset of QuickTime's clock. */
const EPOCH_1904_OFFSET = 2082844800

function quickTimeSeconds(utcMs: number): number {
  return utcMs / 1000 + EPOCH_1904_OFFSET
}

/** A Blob-like source that counts the bytes it was asked to read. */
function fileOf(bytes: Uint8Array, lastModified = 1_700_000_000_000) {
  const source = {
    size: bytes.length,
    lastModified,
    readBytes: 0,
    slice(start: number, end: number = bytes.length) {
      source.readBytes += end - start
      const part = bytes.slice(start, end)
      return { arrayBuffer: async () => part.buffer }
    }
  }
  return source
}

afterEach(() => {
  vi.unstubAllEnvs()
})

/** Runs the wall-clock assertions in a given device time zone (Node applies TZ changes at runtime). */
function inTimeZone(zone: string): void {
  vi.stubEnv('TZ', zone)
}

// ---- JPEG -----------------------------------------------------------------

describe('captureTimeFromJpeg', () => {
  it('reads DateTimeOriginal from a big-endian EXIF block', () => {
    expect(captureTimeFromJpeg(jpeg(tiff(false, { dto: '2026:10:18 18:30:15' })))).toBe('2026-10-18T18:30:15')
  })

  it('reads DateTimeOriginal from a little-endian EXIF block', () => {
    expect(captureTimeFromJpeg(jpeg(tiff(true, { dto: '2026:10:18 18:30:15' })))).toBe('2026-10-18T18:30:15')
  })

  it('falls back to DateTimeDigitized when DateTimeOriginal is absent', () => {
    expect(captureTimeFromJpeg(jpeg(tiff(false, { dtd: '2026:10:17 09:05:00' })))).toBe('2026-10-17T09:05:00')
  })

  it('prefers DateTimeOriginal over DateTimeDigitized', () => {
    expect(captureTimeFromJpeg(jpeg(tiff(true, { dto: '2026:10:18 18:30:15', dtd: '2026:10:19 10:00:00' })))).toBe(
      '2026-10-18T18:30:15'
    )
  })

  it('returns null without an EXIF block or without date tags', () => {
    expect(captureTimeFromJpeg(jpeg())).toBeNull()
    expect(captureTimeFromJpeg(jpeg(tiff(false, {})))).toBeNull()
  })

  it('rejects impossible dates and years outside 2000-2100', () => {
    expect(captureTimeFromJpeg(jpeg(tiff(false, { dto: '2026:02:30 10:00:00' })))).toBeNull()
    expect(captureTimeFromJpeg(jpeg(tiff(false, { dto: '1999:12:31 23:59:59' })))).toBeNull()
    expect(captureTimeFromJpeg(jpeg(tiff(false, { dto: '2101:01:01 00:00:00' })))).toBeNull()
  })

  it('returns null for a non-JPEG or truncated input and never throws', () => {
    expect(captureTimeFromJpeg(enc('not a picture'))).toBeNull()
    const full = jpeg(tiff(false, { dto: '2026:10:18 18:30:15' }))
    for (let length = 0; length < full.length; length++) {
      const result = captureTimeFromJpeg(full.subarray(0, length))
      expect(result === null || /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(result)).toBe(true)
    }
  })
})

// ---- HEIC / HEIF ------------------------------------------------------------

describe('readCaptureTime: HEIC/HEIF', () => {
  it('reads the Exif item located by iloc inside mdat (iloc v0)', async () => {
    const bytes = heic(exifItem(tiff(false, { dto: '2026:10:18 18:30:15' })))
    expect(await readCaptureTime(fileOf(bytes))).toBe('2026-10-18T18:30:15')
  })

  it('reads the Exif item when iloc is version 1 (little-endian EXIF)', async () => {
    const bytes = heic(exifItem(tiff(true, { dtd: '2026:10:18 07:00:01' })), { ilocVersion: 1 })
    expect(await readCaptureTime(fileOf(bytes))).toBe('2026-10-18T07:00:01')
  })

  it('returns null when the file has no Exif item', async () => {
    const bytes = heic(exifItem(tiff(false, {})))
    expect(await readCaptureTime(fileOf(bytes))).toBeNull()
  })

  it('returns null for a truncated HEIC file', async () => {
    const bytes = heic(exifItem(tiff(false, { dto: '2026:10:18 18:30:15' })))
    expect(await readCaptureTime(fileOf(bytes.subarray(0, 60)))).toBeNull()
  })

  it('exposes the raw parser for an in-memory HEIC buffer', () => {
    const bytes = heic(exifItem(tiff(false, { dto: '2026:10:18 18:30:15' })))
    expect(captureTimeFromHeic(bytes)).toBe('2026-10-18T18:30:15')
  })
})

// ---- MP4 / MOV ----------------------------------------------------------------

describe('mp4CreationToLocal', () => {
  it('converts a QuickTime UTC instant to the device wall clock', () => {
    inTimeZone('Europe/Madrid')
    // 16:30 UTC in October is 18:30 in Madrid (CEST, UTC+2).
    expect(mp4CreationToLocal(quickTimeSeconds(Date.UTC(2026, 9, 18, 16, 30, 0)))).toBe('2026-10-18T18:30:00')
  })

  it('follows the device time zone, not a fixed offset', () => {
    inTimeZone('America/New_York')
    // October 18 is EDT (UTC-4) in New York.
    expect(mp4CreationToLocal(quickTimeSeconds(Date.UTC(2026, 9, 18, 16, 30, 0)))).toBe('2026-10-18T12:30:00')
  })

  it('treats zero as absent and rejects years outside 2000-2100', () => {
    expect(mp4CreationToLocal(0)).toBeNull()
    expect(mp4CreationToLocal(quickTimeSeconds(Date.UTC(1999, 5, 1, 12, 0, 0)))).toBeNull()
    expect(mp4CreationToLocal(quickTimeSeconds(Date.UTC(2101, 5, 1, 12, 0, 0)))).toBeNull()
  })
})

describe('captureTimeFromMp4 / readCaptureTime: MP4 and MOV', () => {
  const instant = Date.UTC(2026, 9, 18, 16, 30, 0)

  it('reads mvhd creation_time version 0', async () => {
    inTimeZone('Europe/Madrid')
    const bytes = mp4({ version: 0, seconds: quickTimeSeconds(instant) })
    expect(await readCaptureTime(fileOf(bytes))).toBe('2026-10-18T18:30:00')
    expect(captureTimeFromMp4(bytes)).toBe('2026-10-18T18:30:00')
  })

  it('reads mvhd creation_time version 1 (64-bit)', async () => {
    inTimeZone('Europe/Madrid')
    const bytes = mp4({ version: 1, seconds: quickTimeSeconds(instant) })
    expect(await readCaptureTime(fileOf(bytes))).toBe('2026-10-18T18:30:00')
  })

  it('returns null when creation_time is zero (absent)', async () => {
    expect(await readCaptureTime(fileOf(mp4({ version: 0, seconds: 0 })))).toBeNull()
    expect(await readCaptureTime(fileOf(mp4({ version: 1, seconds: 0 })))).toBeNull()
  })

  it('finds a moov box placed after a large mdat and reads only bounded slices', async () => {
    inTimeZone('Europe/Madrid')
    const size = 4 * 1024 * 1024
    const bytes = mp4({ version: 0, seconds: quickTimeSeconds(instant), moovAtEnd: true, mdatBytes: size })
    const file = fileOf(bytes)
    expect(await readCaptureTime(file)).toBe('2026-10-18T18:30:00')
    // Head (≤ 256 KiB) plus 16-byte box headers and the small moov: nowhere near the 4 MiB payload.
    expect(file.readBytes).toBeLessThan(300 * 1024)
  })

  it('returns null for a truncated or garbled MP4 without throwing', async () => {
    const bytes = mp4({ version: 0, seconds: quickTimeSeconds(instant) })
    for (let length = 0; length < bytes.length; length += 3) {
      const result = await readCaptureTime(fileOf(bytes.subarray(0, length)))
      expect(result === null || /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(result)).toBe(true)
    }
    const garbled = bytes.map((byte, index) => (index % 7 === 0 ? 0xff : byte))
    expect(await readCaptureTime(fileOf(garbled))).toBeNull()
  })
})

// ---- dispatch and guarantees ----------------------------------------------------

describe('readCaptureTime guarantees', () => {
  it('returns null for an unknown format, an empty file, and never uses lastModified', async () => {
    expect(await readCaptureTime(fileOf(enc('plain text'), 1_790_000_000_000))).toBeNull()
    expect(await readCaptureTime(fileOf(new Uint8Array()))).toBeNull()
    expect(await readCaptureTime(fileOf(jpeg(), 1_790_000_000_000))).toBeNull()
  })

  it('reads a JPEG head only, never the whole file', async () => {
    const big = concat(jpeg(tiff(false, { dto: '2026:10:18 18:30:15' })), new Uint8Array(2 * 1024 * 1024))
    const file = fileOf(big)
    expect(await readCaptureTime(file)).toBe('2026-10-18T18:30:15')
    expect(file.readBytes).toBeLessThanOrEqual(256 * 1024)
  })

  it('resolves to null instead of rejecting when the source fails to read', async () => {
    const broken = {
      size: 100,
      slice() {
        throw new Error('disk gone')
      }
    }
    await expect(readCaptureTime(broken)).resolves.toBeNull()
  })
})
