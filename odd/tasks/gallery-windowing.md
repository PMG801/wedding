# Gallery memory cap (bounded progressive loading)
Status: IN PROGRESS. Branch `feat/gallery-windowing` from `develop` (dd49b1f).
Tests: `cd frontend && npm test && npm run build`; backend only if the API changes; `scripts/smoke.sh`.

## Objective
Keep the gallery usable with 500+ photos and 100+ videos: load progressively and cap the number of items held in the browser so memory does not grow unbounded.

## Current state (verified)
- Progressive loading already exists: `GET /api/media?limit=50` with an opaque keyset cursor (`docs/api.md`), `frontend/src/lib/gallery/feed.ts` appends pages, and `frontend/src/components/GalleryGrid.svelte` loads the next page with an `IntersectionObserver` sentinel (`rootMargin: 600px`) and `loading="lazy"` images.
- Missing: nothing is ever released. `GalleryFeed.#items` only grows, so every tile stays in the DOM and in memory (thumbnails, video badges) however far the guest scrolls.

## Decisions (technical, taken by the parent; L1)
- **D1 — Technique.** (a) Windowing: keep the full list of lightweight metadata but render only tiles near the viewport, with spacers for the rest (needs predictable tile height; the grid is square tiles, so feasible). (b) Drop pages far from the viewport and refetch via stored per-page cursors, with spacers. **Chosen: (a)**, because metadata for 600 items is tiny and the cost is DOM and decoded images; no extra requests when scrolling back. Rows are windowed against the window scroll: column count read from the grid's computed `grid-template-columns`, row height = tile width + row gap (tiles are square, the like button is absolutely positioned inside the tile), spacers above and below keep the full height.
- **D2 — Cap.** Maximum rendered tiles (e.g. ~150) and, if (b), maximum pages kept (e.g. 4 x 50). **Chosen:** render the visible rows plus one viewport height of overscan above and below, hard-capped at **150 tiles** (N in S1). On a 390×844 phone with 3 columns this mounts about 60 tiles. A real-phone memory check moves to the deploy rehearsal (runbook T9) instead of blocking T0.
- **D3 — Scroll restoration** when returning from the detail dialog (`#/photo/{id}`) must keep position. **Verified:** `PhotoDialog` is a modal `<dialog>` over the mounted grid, so the window scroll is untouched; windowing must not recompute from a reset scroll while it is open.
- **D4 — Interaction with `gallery-likes`.** Switching between «Recientes» and «Más gustadas» resets the window to the top; the ranking (≤ 50) goes through the same windowing.

## Specs
- **S1** At most N tiles (D2) exist in the DOM at any time, however many items were loaded.
- **S2** Scroll height stays stable (no jumps) while tiles are mounted/unmounted; scrolling back up shows the items again.
- **S3** Page size stays 50; the next page still loads near the end; head refresh after uploads keeps working (existing `feed.test.ts` stays green).
- **S4** Opening and closing a detail view preserves scroll position.
- **S5** Docs: note the cap in `docs/architecture.md`/runbook (Spanish).

## Tasks
- [x] **T0 [D1-D4] — Decide technique and cap** · inline · evidence: decisions above; measurement on a real phone deferred to runbook T9 · commit: this doc
- [ ] **T1 [S1, S2] — Pure windowing logic (visible range from scroll offset, tile size, columns) with tests** · inline, test-first · commit: pending
- [ ] **T2 [S1-S4] — Integrate into `GalleryGrid.svelte`; keep feed tests green** · inline · commit: pending
- [ ] **T3 [S5] — Docs and a check with a large seeded gallery in smoke** · inline · commit: pending

Risk: none expected (frontend only, existing feed tests cover regressions); revisit if the API changes.

## Log
- **L1** (user, 2026-10-18): "continua con la siguiente implementacion, pero antes quiero que merges los cambios a develop y hagas una limpieza de ramas"
- **L2** Before starting: `feat/gallery-likes-smoke` fast-forwarded into `develop` and pushed (dd49b1f); all merged local and remote branches deleted except `main`/`develop`. CI watch unavailable (`gh` not authenticated).
