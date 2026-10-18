/** Hard cap on tiles mounted at once (D2 / S1). */
export const MAX_RENDERED_TILES = 150

export interface WindowRequest {
  /** Distance in px from the top of the grid to the top of the viewport (clamped to the grid). */
  scrollOffset: number
  viewportHeight: number
  /** Row pitch in px: tile height plus row gap. */
  rowHeight: number
  columns: number
  total: number
  /** Extra px rendered above and below the viewport. Defaults to one viewport. */
  overscan?: number
  maxTiles?: number
}

export interface TileWindow {
  /** Index of the first item to render; always a multiple of `columns` unless capped by `maxTiles`. */
  start: number
  /** Index after the last item to render. */
  end: number
  /** Height in px of the spacer above the rendered rows (rows are `rowHeight` pitch). */
  topSpacer: number
  /** Height in px of the spacer below the rendered rows. */
  bottomSpacer: number
}

const EMPTY: TileWindow = { start: 0, end: 0, topSpacer: 0, bottomSpacer: 0 }

/**
 * Returns the item range to mount for the current scroll position, plus the
 * spacer heights that stand in for the unmounted rows. Rows are whole; the
 * window covers the viewport plus overscan, trimmed evenly to `maxTiles`.
 * Without a usable row height (no layout, e.g. jsdom) it returns a bounded
 * first window and no spacers.
 */
export function computeWindow(request: WindowRequest): TileWindow {
  const total = Number.isFinite(request.total) ? Math.floor(request.total) : 0
  if (total <= 0) return EMPTY

  const maxTiles = Math.max(1, Math.floor(request.maxTiles ?? MAX_RENDERED_TILES))
  // A list that fits under the cap (e.g. the ranking) is never windowed.
  if (total <= maxTiles) return { start: 0, end: total, topSpacer: 0, bottomSpacer: 0 }

  const columns = Math.max(1, Math.floor(request.columns) || 1)
  const rowHeight = request.rowHeight
  if (!Number.isFinite(rowHeight) || rowHeight <= 0) {
    return { start: 0, end: maxTiles, topSpacer: 0, bottomSpacer: 0 }
  }

  const totalRows = Math.ceil(total / columns)
  const viewportHeight = Number.isFinite(request.viewportHeight) ? Math.max(0, request.viewportHeight) : 0
  const overscan = Math.max(0, request.overscan ?? viewportHeight)
  const scroll = Math.min(Math.max(0, Number.isFinite(request.scrollOffset) ? request.scrollOffset : 0), totalRows * rowHeight)

  const viewTop = Math.min(totalRows - 1, Math.floor(scroll / rowHeight))
  const viewBottom = Math.min(totalRows - 1, Math.floor((scroll + viewportHeight) / rowHeight))
  let first = Math.max(0, Math.floor((scroll - overscan) / rowHeight))
  let last = Math.min(totalRows - 1, Math.floor((scroll + viewportHeight + overscan) / rowHeight))

  // Enforce the cap: trim overscan evenly, never the rows the viewport shows.
  const maxRows = Math.max(1, Math.floor(maxTiles / columns))
  let excess = last - first + 1 - maxRows
  if (excess > 0) {
    const trimTop = Math.min(viewTop - first, Math.ceil(excess / 2))
    first += trimTop
    excess -= trimTop
    const trimBottom = Math.min(last - viewBottom, excess)
    last -= trimBottom
    excess -= trimBottom
    // The viewport alone is taller than the cap: keep its top rows.
    last -= excess
  }

  const start = first * columns
  const end = Math.min(total, (last + 1) * columns, start + maxTiles)
  const renderedRowsEnd = Math.ceil(end / columns)
  return {
    start,
    end,
    topSpacer: first * rowHeight,
    bottomSpacer: (totalRows - renderedRowsEnd) * rowHeight
  }
}
