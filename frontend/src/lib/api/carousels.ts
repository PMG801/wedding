import { GuestSessionRequiredError, parseGalleryItem, parseGalleryPage, type GalleryItem, type GalleryPage } from './media'

/** One time period of the story strip: its wall-clock range and the items of the current rotation slot. */
export interface CarouselPeriod {
  id: number
  /** Wall clock `YYYY-MM-DDTHH:MM[:SS]` of the start, inclusive. */
  start: string
  /** Wall clock of the end, exclusive. */
  end: string
  /** Visible media in the period, not only the ones shown now. */
  total: number
  items: GalleryItem[]
}

export interface Carousels {
  /** Unix ms of the next rotation slot; the strip refetches then. */
  rotatesAt: number
  periods: CarouselPeriod[]
}

export class PeriodNotFoundError extends Error {
  constructor() {
    super('The period does not exist')
    this.name = 'PeriodNotFoundError'
  }
}

const CAROUSEL_INIT: RequestInit = { credentials: 'same-origin', cache: 'no-store' }
const INVALID_STRIP = 'Carousels response has an invalid shape'

/** The story strip: every period with media, and the next rotation instant (GET /api/carousels). */
export async function fetchCarousels({ fetchFn = fetch }: { fetchFn?: typeof fetch } = {}): Promise<Carousels> {
  const response = await fetchFn('/api/carousels', CAROUSEL_INIT)
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (!response.ok) throw new Error(`Carousels request failed with HTTP ${response.status}`)

  const body: unknown = await response.json()
  if (!isRecord(body) || !Number.isInteger(body.rotates_at) || !Array.isArray(body.periods)) {
    throw new Error(INVALID_STRIP)
  }
  return { rotatesAt: body.rotates_at as number, periods: body.periods.map(parsePeriod) }
}

/** One page of a period's full album, in capture-time order (GET /api/carousels/{id}). */
export async function fetchPeriodPage({
  id,
  cursor = null,
  limit = 50,
  fetchFn = fetch
}: {
  id: number
  cursor?: string | null
  limit?: number
  fetchFn?: typeof fetch
}): Promise<GalleryPage> {
  let url = `/api/carousels/${id}?limit=${limit}`
  if (cursor !== null) url += `&cursor=${encodeURIComponent(cursor)}`
  const response = await fetchFn(url, CAROUSEL_INIT)
  if (response.status === 401) throw new GuestSessionRequiredError()
  if (response.status === 404) throw new PeriodNotFoundError()
  if (!response.ok) throw new Error(`Carousel page request failed with HTTP ${response.status}`)
  return parseGalleryPage(await response.json(), 'Carousel page response has an invalid shape')
}

function parsePeriod(raw: unknown): CarouselPeriod {
  if (
    !isRecord(raw) ||
    !Number.isInteger(raw.id) ||
    (raw.id as number) < 1 ||
    typeof raw.start !== 'string' ||
    typeof raw.end !== 'string' ||
    !Number.isInteger(raw.total) ||
    (raw.total as number) < 0 ||
    !Array.isArray(raw.items)
  ) {
    throw new Error(INVALID_STRIP)
  }
  return {
    id: raw.id as number,
    start: raw.start,
    end: raw.end,
    total: raw.total as number,
    items: raw.items.map((item) => parseGalleryItem(item, INVALID_STRIP))
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}
