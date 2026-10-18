# Gallery likes and popularity ranking
Status: IN PROGRESS (authorized L6). Stacked branches from `feat/admin-moderation-docs`, each from the previous one; only the last branch is merged: `feat/gallery-likes-contract` (T0) → `feat/gallery-likes-storage` (T1) → `feat/gallery-likes-api` (T2-T3) → `feat/gallery-likes-ui` (T4) → `feat/gallery-likes-smoke` (T5, merge this last branch).
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`. Sources of truth: `docs/api.md`, `docs/data-model.md`, ADR-0007, ADR-0012.

## Specs
- **S1** The signed guest cookie carries a stable anonymous `guest_id` (lazily assigned to an existing valid cookie). A unique `(guest_id, media_id)` like row means one like from that guest identity per medium; repeating the request is idempotent and cannot increase the count again. A distinct guest cookie is a distinct voter. User L3: "que solo pueda dar varios likes si se limpia la cache o entra desde otro navegador o otro dispositivo o caduca la cookie".
- **S2** Store likes durably in SQLite. Persist aggregate `media.like_count` transactionally with the like table so counts survive restarts and cannot drift; no in-memory count cache is the source of truth. User L3: "si van en la bbdd no quiero perder la contabilizacion".
- **S3** The chronological gallery keeps its existing cursor and adds `like_count` and `liked_by_me`. A separate ranking endpoint returns at most 50 visible items in deterministic popularity order (`like_count DESC, confirmed_at DESC, id DESC`) in one response, without a cursor or page continuation. Only items with at least one like are listed (L5: "exacto minimo un like"). User L3: "que el filtro ese no sea de todas las fotos sino un rankig es decir cargar los 50 elementos mas gustados".
- **S4** Hidden media cannot be liked and never appears in the chronological gallery or top-50 ranking.
- **S5** UI: like button in grid tile and detail dialog, optimistic update with rollback on error; Spanish copy; separate "Recientes" and "Más gustadas" views.
- **S6** Docs: `docs/api.md`, `docs/data-model.md`, new ADR-0012 (Spanish).
- **S7** Do not add an in-memory request rate limit unless separately authorized; no `429 rate_limited` behavior is currently specified.

## Decisions
- **D1 — Identity: RESOLVED (L2).** A stable anonymous `guest_id` is stored in the signed guest cookie; an existing valid cookie receives one lazily. Uniqueness is `(guest_id, media_id)`. The same cookie cannot add another like to a medium. A new guest identity (cleared browser cookie/cache, another browser/device, or a cookie expired after 30 days) may like it independently. Likes from distinct devices are intentionally not deduplicated. The cookie lifetime follows ADR-0007; ADR-0012 records this feature decision.
- **D2 — Model: RESOLVED (L2).** Positive likes only, with like/unlike actions; no downvotes.
- **D3 — Default view: RESOLVED (L3).** The normal gallery remains chronological ("Recientes"). "Más gustadas" is a separate ranking view, not a reorder of the chronological feed.
- **D4 — Ranking pagination and minimum count: RESOLVED (L3-L5).** "Más gustadas" returns at most the top 50 visible items in one response and has no cursor or subsequent pages. Ranking ties are ordered by `confirmed_at DESC, id DESC`. Every listed medium must have at least one like; zero-like items are excluded.
- **D5 — Durable counting: RESOLVED (L3).** Likes and their aggregate counts are persisted in SQLite and survive application restarts. Enforce one stored like per `(guest_id, media_id)`; a repeated like from the same cookie is idempotent and does not increment the count. Never reset persisted counts as a cache operation. No additional in-memory rate-limit policy has been requested.
- **D6 — Cookie and contract (technical, derived from D1/L3).** Cookie v2 payload = 8-byte expiry + 16 random bytes `guest_id` (32 lowercase hex chars), HMAC under a new domain label `boda-guest-session-v2:`. A valid v1 cookie is still accepted and, on any guest-protected request, is upgraded to v2 with a fresh `guest_id` **keeping its original expiry** (no lifetime extension). Re-entering through the QR (`/e/{token}`) with a valid cookie keeps its `guest_id` (new expiry), so rescanning in the same browser does not create a new voter. Endpoints: `PUT`/`DELETE /api/media/{id}/like` → `200 {"id","like_count","liked_by_me"}`; `GET /api/media/top` → `{"items":[...]}` (max 50, `like_count >= 1`). Hidden or unknown media → `404 media_not_found`. Likes are not gated by the uploads pause switch. Contract details in `docs/api.md`.

## Tasks
- [ ] **T0 [S6, D1-D6] — Record decisions: ADR-0012, API contract, data model** · inline · branch `feat/gallery-likes-contract` · commit: pending
- [ ] **T1 [S1, S2, S4] — Cookie `guest_id` (v2 + lazy upgrade), migration `0002` `media_likes` + `like_count`, like store** · backend worker, test-first · risk: item 1 (schema migration) + item 2 (session cookie) → own commit, independent verify · branch `feat/gallery-likes-storage` · commit: pending
- [ ] **T2 [S1, S4] — `PUT/DELETE /api/media/{id}/like`** · backend worker, test-first · branch `feat/gallery-likes-api` · commit: pending
- [ ] **T3 [S3, S4] — `like_count`/`liked_by_me` in the gallery and `GET /api/media/top`** · backend worker, test-first; existing chronological cursor tests stay green · branch `feat/gallery-likes-api` · commit: pending
- [ ] **T4 [S5] — Frontend like button and "Recientes"/"Más gustadas" views** · frontend worker (parallel, against the T0 contract), test-first · branch `feat/gallery-likes-ui` · commit: pending
- [ ] **T5 [S6] — Runbook note and smoke coverage** · inline · branch `feat/gallery-likes-smoke` · commit: pending

## Dependencies
Admin hiding (`admin-moderation`) affects S4; it is implemented on the base branch.

## Log
- **L3** (user, 2026-10-18): "no empieces aun con gallery likes, para solucionar el tema de paginacion lo que vamos a hacer es que el filtro ese no sea de todas las fotos sino un rankig es decir cargar los 50 elementos mas gustados. Para el tema de contabilizar los likes si van en la bbdd no quiero perder la contabilizacion y que solo pueda dar varios likes si se limpia la cache o entra desde otro navegador o otro dispositivo o caduca la cookie"
- **L4** (user, 2026-10-18): "acaba de rellenar los datos sin empezar la accion"
- **L5** (user, 2026-10-18): "exacto minimo un like"
- **L6** (user, 2026-10-18): "Vamos con la implementacion pendiente de los likes, recuerda aprovechar todas las herramientas de gentle como los subagentes"
