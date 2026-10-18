# Gallery likes and popularity ordering
Status: PLANNED (not started). Branch when started: `feat/gallery-likes` from `develop`.
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`.

## Objective
Let guests mark photos/videos they like so the gallery can surface the most liked ones, instead of an unmanageable chronological wall.

## Decisions (user, L2)
- **D1 — Identity: RESOLVED.** Multiple votes from different devices/sessions are acceptable. Only abuse from the same browser session must be limited. Implementation: anonymous `guest_id` inside the signed guest cookie (issued lazily to existing cookies); one like per `(guest_id, media)`; plus a per-session rate limit on like requests. The cookie already expires (30 days, ADR-0007). Needs a short ADR-0012 note.
- **D2 — Model: RESOLVED.** Positive likes only (toggle like/unlike), no downvotes.
- **D3 — Default order.** Open. Recommendation: switch "Recientes" / "Más gustadas", default recent.
- **D4 — Cursor.** Open. Popularity order needs keyset `(like_count DESC, confirmed_at DESC, id DESC)`; counts change between pages, so pages may shift. Recommendation: accept and document.
- **D5 — Rate limit.** Open: in-memory per `guest_id` (e.g. N likes/minute, `429 rate_limited`), reset on restart. Recommendation: in-memory, no new table.

## Specs (draft, to be fixed after D1-D4)
- **S1** One like per `guest_id` per media; liking twice is idempotent; unliking removes it.
- **S1b** Like requests from one session beyond the rate limit get `429 {"error":"rate_limited"}` and change nothing.
- **S2** `media.like_count` denormalized, kept consistent with the likes table in one transaction.
- **S3** Gallery API accepts an order parameter; invalid value gives `400`; response includes `like_count` and `liked_by_me`.
- **S4** Hidden media cannot be liked and never appear in either order.
- **S5** UI: like button in grid tile and detail dialog, optimistic update with rollback on error; Spanish copy.
- **S6** Docs: `docs/api.md`, `docs/data-model.md`, new ADR (Spanish).

## Tasks
- [ ] **T0 [D3-D5] — Decide and write ADR-0012** · route: user decision + inline · commit: pending
- [ ] **T1 [S1, S2, S4] — Migration `media_likes` + `like_count` + store** · route: inline, test-first · risk: item 1 (schema migration) → own commit · commit: pending
- [ ] **T2 [S1, S4] — `PUT/DELETE /api/media/{id}/like`** · inline, test-first · commit: pending
- [ ] **T3 [S3] — Gallery order parameter, counts and keyset cursor** · inline, test-first; existing chronological cursor test must stay green · commit: pending
- [ ] **T4 [S5] — Frontend like button and order switch** · inline · commit: pending
- [ ] **T5 [S6] — Docs, smoke coverage** · inline · commit: pending

## Dependencies
Admin hiding (T-moderation) affects S4; implement `admin-moderation` first.
