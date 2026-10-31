/**
 * Capture time of a photo or video, read from its metadata before upload (S2).
 * Returns the local wall clock `YYYY-MM-DDTHH:MM:SS` (no zone) or null. Never
 * File.lastModified: a file copied or edited later has the wrong time.
 */

/** Bytes read from the start of a file: JPEG EXIF and HEIC/MP4 headers live here. */
const HEAD_BYTES = 256 * 1024
/** Largest moov box read (long videos keep the sample tables there). */
const MOOV_MAX_BYTES = 16 * 1024 * 1024
/** Largest HEIC meta box read (item tables, a few KB in practice). */
const META_MAX_BYTES = 1024 * 1024
/** Largest Exif item read from a HEIC file (an APP1 segment is at most 64 KiB). */
const EXIF_ITEM_MAX_BYTES = 128 * 1024
/** Guards the box walks against files that loop or nest absurdly. */
const MAX_BOXES = 256
/** Seconds between 1904-01-01 (QuickTime epoch) and 1970-01-01. */
const QUICKTIME_EPOCH_OFFSET_S = 2082844800

/** The part of Blob/File the reader needs, so tests can pass byte fixtures. */
export interface ByteSource {
  readonly size: number
  slice(start: number, end: number): { arrayBuffer(): Promise<ArrayBuffer> }
}

/** Reads a file's capture time. Resolves to null for any unknown, absent or malformed input. */
export async function readCaptureTime(file: ByteSource): Promise<string | null> {
  try {
    if (file.size <= 0) return null
    const head = await readRange(file, 0, Math.min(file.size, HEAD_BYTES))
    if (head[0] === 0xff && head[1] === 0xd8) return captureTimeFromJpeg(head)

    // ISO BMFF (HEIC, MP4, MOV): the top-level boxes say where the metadata is; the file may be large.
    const box = await findTopLevelBox(file)
    if (box === null) return null
    if (box.type === 'moov') {
      if (box.size > MOOV_MAX_BYTES) return null
      return captureTimeFromMoov(await readRange(file, box.start, box.size))
    }
    if (box.size > META_MAX_BYTES) return null
    const location = exifLocation(await readRange(file, box.start, box.size))
    if (location === null || location.length > EXIF_ITEM_MAX_BYTES || location.offset + location.length > file.size) return null
    return exifFromItem(await readRange(file, location.offset, location.length))
  } catch {
    return null
  }
}

/** JPEG: the APP1 segment holding "Exif\0\0" and a TIFF block. Stops at the image data. */
export function captureTimeFromJpeg(bytes: Uint8Array): string | null {
  try {
    if (bytes[0] !== 0xff || bytes[1] !== 0xd8) return null
    let pos = 2
    while (pos + 4 <= bytes.length) {
      if (bytes[pos] !== 0xff) return null
      const marker = bytes[pos + 1]
      if (marker === 0xff) {
        pos += 1
        continue
      }
      if (marker === 0xd8 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
        pos += 2
        continue
      }
      // Start of scan or end of image: the header segments are over.
      if (marker === 0xda || marker === 0xd9) return null
      const length = (bytes[pos + 2] << 8) | bytes[pos + 3]
      const end = pos + 2 + length
      if (length < 2 || end > bytes.length) return null
      if (marker === 0xe1 && startsWith(bytes, pos + 4, 'Exif\0\0')) return exifDateTime(bytes.subarray(pos + 10, end))
      pos = end
    }
    return null
  } catch {
    return null
  }
}

/** HEIC/HEIF in memory: the Exif item is located through meta → iloc and read from the same buffer. */
export function captureTimeFromHeic(bytes: Uint8Array): string | null {
  try {
    const meta = findChild(bytes, 0, bytes.length, 'meta')
    if (meta === null) return null
    const location = exifLocation(bytes.subarray(meta.start, meta.start + meta.size))
    if (location === null || location.offset + location.length > bytes.length) return null
    return exifFromItem(bytes.subarray(location.offset, location.offset + location.length))
  } catch {
    return null
  }
}

/** MP4/MOV in memory: the moov box is found at the top level, wherever it sits. */
export function captureTimeFromMp4(bytes: Uint8Array): string | null {
  try {
    const moov = findChild(bytes, 0, bytes.length, 'moov')
    return moov === null ? null : captureTimeFromMoov(bytes.subarray(moov.start, moov.start + moov.size))
  } catch {
    return null
  }
}

/**
 * Converts an mvhd creation time (seconds since 1904 UTC) to the device's local
 * wall clock. Zero means the camera wrote no time. Years outside 2000-2100 are rejected.
 */
export function mp4CreationToLocal(seconds: number): string | null {
  if (!Number.isSafeInteger(seconds) || seconds <= 0) return null
  const date = new Date((seconds - QUICKTIME_EPOCH_OFFSET_S) * 1000)
  if (Number.isNaN(date.getTime())) return null
  return formatWallClock([
    date.getFullYear(),
    date.getMonth() + 1,
    date.getDate(),
    date.getHours(),
    date.getMinutes(),
    date.getSeconds()
  ])
}

// ---- TIFF / EXIF ------------------------------------------------------------

const TAG_EXIF_IFD_POINTER = 0x8769
const TAG_DATE_TIME_ORIGINAL = 0x9003
const TAG_DATE_TIME_DIGITIZED = 0x9004
const TYPE_ASCII = 2
const TYPE_LONG = 4

interface IfdEntry {
  type: number
  count: number
  /** Position of the 4-byte value/offset field. */
  valueAt: number
}

/** Reads DateTimeOriginal, falling back to DateTimeDigitized, from a TIFF block. */
function exifDateTime(tiff: Uint8Array): string | null {
  try {
    const le = tiff[0] === 0x49 && tiff[1] === 0x49
    if (!le && !(tiff[0] === 0x4d && tiff[1] === 0x4d)) return null
    const header = new Reader(tiff, 2, le)
    if (header.u16() !== 42) return null
    const ifd0 = readIfd(tiff, header.u32(), le)
    const pointer = ifd0.get(TAG_EXIF_IFD_POINTER)
    if (pointer === undefined || pointer.type !== TYPE_LONG) return null
    const exif = readIfd(tiff, new Reader(tiff, pointer.valueAt, le).u32(), le)
    for (const tag of [TAG_DATE_TIME_ORIGINAL, TAG_DATE_TIME_DIGITIZED]) {
      const entry = exif.get(tag)
      const parsed = entry === undefined ? null : parseExifDate(readAscii(tiff, entry, le))
      if (parsed !== null) return parsed
    }
    return null
  } catch {
    return null
  }
}

function readIfd(tiff: Uint8Array, offset: number, le: boolean): Map<number, IfdEntry> {
  const reader = new Reader(tiff, offset, le)
  const entries = new Map<number, IfdEntry>()
  const count = reader.u16()
  for (let i = 0; i < count; i++) {
    const tag = reader.u16()
    const type = reader.u16()
    const entryCount = reader.u32()
    const valueAt = reader.pos
    reader.skip(4)
    entries.set(tag, { type, count: entryCount, valueAt })
  }
  return entries
}

function readAscii(tiff: Uint8Array, entry: IfdEntry, le: boolean): string | null {
  if (entry.type !== TYPE_ASCII) return null
  // Values of up to four bytes sit inline in the entry; longer ones are at an offset.
  const start = entry.count <= 4 ? entry.valueAt : new Reader(tiff, entry.valueAt, le).u32()
  if (start + entry.count > tiff.length) return null
  const raw = tiff.subarray(start, start + entry.count)
  const nul = raw.indexOf(0)
  return String.fromCharCode(...(nul < 0 ? raw : raw.subarray(0, nul)))
}

/** EXIF date text "YYYY:MM:DD HH:MM:SS" to the wall-clock format, or null. */
function parseExifDate(text: string | null): string | null {
  const match = text === null ? null : /^(\d{4}):(\d{2}):(\d{2}) (\d{2}):(\d{2}):(\d{2})/.exec(text)
  return match === null ? null : formatWallClock(match.slice(1).map(Number))
}

// ---- HEIC ---------------------------------------------------------------------

/** Exif item payload: u32 offset from the item start to the TIFF header (6 bytes of "Exif\0\0" first). */
function exifFromItem(item: Uint8Array): string | null {
  if (item.length < 4) return null
  const tiffStart = 4 + new Reader(item).u32()
  return exifDateTime(item.subarray(tiffStart))
}

/**
 * Where the Exif item's bytes sit in the file (absolute offset, length), from a
 * whole meta box. Only single-extent items stored in the file are accepted.
 */
function exifLocation(metaBox: Uint8Array): { offset: number; length: number } | null {
  const meta = boxHeader(metaBox, 0, metaBox.length)
  if (meta === null) return null
  const body = meta.start + meta.header + 4 // FullBox: version and flags
  const iinf = findChild(metaBox, body, meta.start + meta.size, 'iinf')
  const iloc = findChild(metaBox, body, meta.start + meta.size, 'iloc')
  if (iinf === null || iloc === null) return null
  const itemId = exifItemId(metaBox, iinf)
  return itemId === null ? null : ilocExtent(metaBox, iloc, itemId)
}

function exifItemId(bytes: Uint8Array, iinf: Box): number | null {
  const reader = new Reader(bytes, iinf.start + iinf.header)
  const version = reader.u8()
  reader.skip(3)
  reader.skip(version === 0 ? 2 : 4) // entry_count
  for (const child of children(bytes, reader.pos, iinf.start + iinf.size)) {
    if (child.type !== 'infe') continue
    const infe = new Reader(bytes, child.start + child.header)
    const infeVersion = infe.u8()
    infe.skip(3)
    if (infeVersion < 2) continue
    const id = infeVersion === 2 ? infe.u16() : infe.u32()
    infe.skip(2) // item_protection_index
    if (infe.ascii(4) === 'Exif') return id
  }
  return null
}

function ilocExtent(bytes: Uint8Array, iloc: Box, itemId: number): { offset: number; length: number } | null {
  const reader = new Reader(bytes, iloc.start + iloc.header)
  const version = reader.u8()
  reader.skip(3)
  const sizes = reader.u8()
  const offsetSize = sizes >> 4
  const lengthSize = sizes & 0x0f
  const baseSizes = reader.u8()
  const baseOffsetSize = baseSizes >> 4
  const indexSize = version === 1 || version === 2 ? baseSizes & 0x0f : 0
  const itemCount = version < 2 ? reader.u16() : reader.u32()
  for (let i = 0; i < itemCount; i++) {
    const id = version < 2 ? reader.u16() : reader.u32()
    const constructionMethod = version >= 1 ? reader.u16() & 0x0f : 0
    reader.skip(2) // data_reference_index
    const baseOffset = reader.uint(baseOffsetSize)
    const extentCount = reader.u16()
    const extents: { offset: number; length: number }[] = []
    for (let e = 0; e < extentCount; e++) {
      reader.uint(indexSize)
      extents.push({ offset: reader.uint(offsetSize), length: reader.uint(lengthSize) })
    }
    if (id !== itemId) continue
    if (constructionMethod !== 0 || extents.length !== 1) return null
    return { offset: baseOffset + extents[0].offset, length: extents[0].length }
  }
  return null
}

// ---- MP4 / MOV ----------------------------------------------------------------

function captureTimeFromMoov(moov: Uint8Array): string | null {
  const header = boxHeader(moov, 0, moov.length)
  if (header === null) return null
  const mvhd = findChild(moov, header.start + header.header, moov.length, 'mvhd')
  if (mvhd === null) return null
  const reader = new Reader(moov, mvhd.start + mvhd.header)
  const version = reader.u8()
  reader.skip(3)
  return mp4CreationToLocal(version === 1 ? reader.u64() : reader.u32())
}

// ---- ISO BMFF boxes -------------------------------------------------------------

interface Box {
  type: string
  start: number
  size: number
  header: number
}

/** Parses one box header at `start`; `head` holds its first bytes. Null when the box runs past `end`. */
function boxHeader(head: Uint8Array, start: number, end: number): Box | null {
  const reader = new Reader(head)
  let size = reader.u32()
  const type = reader.ascii(4)
  let header = 8
  if (size === 1) {
    size = reader.u64()
    header = 16
  } else if (size === 0) {
    size = end - start
  }
  if (size < header || start + size > end) return null
  return { type, start, size, header }
}

function children(bytes: Uint8Array, from: number, to: number): Box[] {
  const boxes: Box[] = []
  let pos = from
  while (boxes.length < MAX_BOXES && pos + 8 <= to) {
    const box = boxHeader(bytes.subarray(pos, Math.min(to, pos + 16)), pos, to)
    if (box === null) break
    boxes.push(box)
    pos += box.size
  }
  return boxes
}

function findChild(bytes: Uint8Array, from: number, to: number, type: string): Box | null {
  return children(bytes, from, to).find((box) => box.type === type) ?? null
}

/** The first top-level meta or moov box of a file, read through bounded slices. */
async function findTopLevelBox(file: ByteSource): Promise<Box | null> {
  let pos = 0
  for (let i = 0; i < MAX_BOXES && pos + 8 <= file.size; i++) {
    const head = await readRange(file, pos, Math.min(16, file.size - pos))
    const box = boxHeader(head, pos, file.size)
    if (box === null) return null
    if (box.type === 'moov' || box.type === 'meta') return box
    pos += box.size
  }
  return null
}

async function readRange(file: ByteSource, start: number, length: number): Promise<Uint8Array> {
  return new Uint8Array(await file.slice(start, start + length).arrayBuffer())
}

// ---- helpers ----------------------------------------------------------------------

/** Bounds-checked big- or little-endian reader; any read past the end throws (callers return null). */
class Reader {
  readonly #view: DataView
  pos: number

  constructor(
    readonly bytes: Uint8Array,
    pos = 0,
    readonly le = false
  ) {
    this.#view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
    this.pos = pos
  }

  #take(count: number): number {
    if (this.pos < 0 || this.pos + count > this.bytes.length) throw new RangeError('truncated')
    const at = this.pos
    this.pos += count
    return at
  }

  skip(count: number): void {
    this.#take(count)
  }

  u8(): number {
    return this.bytes[this.#take(1)]
  }

  u16(): number {
    return this.#view.getUint16(this.#take(2), this.le)
  }

  u32(): number {
    return this.#view.getUint32(this.#take(4), this.le)
  }

  u64(): number {
    const value = this.#view.getBigUint64(this.#take(8), this.le)
    if (value > BigInt(Number.MAX_SAFE_INTEGER)) throw new RangeError('too large')
    return Number(value)
  }

  /** An unsigned field of 0, 4 or 8 bytes, as the ISO BMFF sizes allow. */
  uint(size: number): number {
    if (size === 0) return 0
    if (size === 4) return this.u32()
    if (size === 8) return this.u64()
    throw new RangeError('unsupported field size')
  }

  ascii(count: number): string {
    const at = this.#take(count)
    return String.fromCharCode(...this.bytes.subarray(at, at + count))
  }
}

function startsWith(bytes: Uint8Array, at: number, text: string): boolean {
  for (let i = 0; i < text.length; i++) {
    if (bytes[at + i] !== text.charCodeAt(i)) return false
  }
  return true
}

/** [year, month, day, hour, minute, second] to `YYYY-MM-DDTHH:MM:SS`, or null when impossible or outside 2000-2100. */
function formatWallClock([year, month, day, hour, minute, second]: number[]): string | null {
  if (year < 2000 || year > 2100 || month < 1 || month > 12 || hour > 23 || minute > 59 || second > 59) return null
  const daysInMonth = new Date(Date.UTC(year, month, 0)).getUTCDate()
  if (day < 1 || day > daysInMonth) return null
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${year}-${pad(month)}-${pad(day)}T${pad(hour)}:${pad(minute)}:${pad(second)}`
}
