# Gallery memory cap (bounded progressive loading)
Status: PLANNED (not started). Branch when started: `fix/gallery-windowing` or `feat/gallery-windowing` from `develop`.
Tests: `cd frontend && npm test && npm run build`; backend only if the API changes; `scripts/smoke.sh`.

## Objective
Keep the gallery usable with 500+ photos and 100+ videos: load progressively and cap the number of items held in the browser so memory does not grow unbounded.

## Current state (verified)
- Progressive loading already exists: `GET /api/media?limit=50` with an opaque keyset cursor (`docs/api.md`), `frontend/src/lib/gallery/feed.ts` appends pages, and `frontend/src/components/GalleryGrid.svelte` loads the next page with an `IntersectionObserver` sentinel (`rootMargin: 600px`) and `loading="lazy"` images.
- Missing: nothing is ever released. `GalleryFeed.#items` only grows, so every tile stays in the DOM and in memory (thumbnails, video badges) however far the guest scrolls.

## Open decisions
- **D1 — Technique.** (a) Windowing: keep the full list of lightweight metadata but render only tiles near the viewport, with spacers for the rest (needs predictable tile height; the grid is square tiles, so feasible). (b) Drop pages far from the viewport and refetch via stored per-page cursors, with spacers. Recommendation: (a), because metadata for 600 items is tiny and the cost is DOM and decoded images; no extra requests when scrolling back.
- **D2 — Cap.** Maximum rendered tiles (e.g. ~150) and, if (b), maximum pages kept (e.g. 4 x 50). Needs a measurement on a real phone before fixing numbers.
- **D3 — Scroll restoration** when returning from the detail dialog (`#/photo/{id}`) must keep position.
- **D4 — Interaction with `gallery-likes`.** A different order resets the feed and windowing state.

## Specs (draft)
- **S1** At most N tiles (D2) exist in the DOM at any time, however many items were loaded.
- **S2** Scroll height stays stable (no jumps) while tiles are mounted/unmounted; scrolling back up shows the items again.
- **S3** Page size stays 50; the next page still loads near the end; head refresh after uploads keeps working (existing `feed.test.ts` stays green).
- **S4** Opening and closing a detail view preserves scroll position.
- **S5** Docs: note the cap in `docs/architecture.md`/runbook (Spanish).

## Tasks
- [ ] **T0 [D1-D3] — Measure and decide technique and cap** (500 photos + 100 videos fixture, mobile emulation, memory/DOM node counts) · inline · commit: pending
- [ ] **T1 [S1, S2] — Pure windowing logic (visible range from scroll offset, tile size, columns) with tests** · inline, test-first · commit: pending
- [ ] **T2 [S1-S4] — Integrate into `GalleryGrid.svelte`; keep feed tests green** · inline · commit: pending
- [ ] **T3 [S5] — Docs and a check with a large seeded gallery in smoke** · inline · commit: pending

Risk: none expected (frontend only, existing feed tests cover regressions); revisit if the API changes.
