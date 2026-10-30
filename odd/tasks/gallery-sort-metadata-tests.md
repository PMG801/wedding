# Gallery order toggle, metadata-driven albums and extra tests
Status: IN PROGRESS. T1 implemented on `feat/gallery-order-toggle`; T4 stacked on it; T5 on its own branch; T2/T3 blocked on D2 (user decision). Branch names are proposals.
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`. Sources of truth: `docs/api.md`, `docs/data-model.md`, ADR-0004, ADR-0007, ADR-0012; discussion `odd/discussions/gallery-search-and-ordering.md`.

## Specs
- **S1** Gallery order filter from most recent to oldest (and back). L1: "algo mas de funcionalidad como la de un filtro de mas recientes a mas antiguas".
- **S2** Metadata of media is stored so that "algunos albunes o carrusels" can be managed autonomously. L1: "poder jugar con el tema de la metadata ya que se podrian gestionar de forma autonoma algunos albunes o carrusels".
- **S3** More tests, meaning stress/performance/stability scenarios, not code tests. L2: "mas test no me refiero de codigo si no ideas o pruebas con datos de rendiiento o estabilidad de la app ante diferentes estres ya sea por concurrencia, por saturacion o mala red quiero ver diferentes casos para ver fallos y que se puede mejorar".
- **S5** Filter by media type: only photos or only videos. L2: "El otro filtro podria ser filtrar por solo fotos o videos".
- **S6** Oldest/newest order is separate from "Más gustadas". L2: "El tema de ordenar de viejo a nuevo o al reves debe estar separado de los gustados".
- **S4** After S1-S3 are planned, debate with the user what else to offer for ordering (not a task; see `odd/discussions/gallery-search-and-ordering.md`).

## Open decisions (ask before implementing)
- **D1 — RESOLVED (L2, S6).** Order toggle (recent↔oldest) applies to the chronological feed only; "Más gustadas" stays separate. Technical: needs an ascending keyset cursor and index. Open: does the type filter (S5) compose with the order toggle? Proposed yes (query params `order`, `type`).
- **D2** S2: which metadata? Candidates: capture time (EXIF/`lastModified`, client-side per ADR-0004), media type, dimensions, duration, orientation. Which albums/carousels? E.g. by moment of day, photos/videos, "momentos" derived automatically. Server-derived or client-sent at upload (trust and idempotency, ADR-0005)? Needs a migration (high risk item 1 → own commit).
- **D3 — RESOLVED (L2).** S3 is a stress/stability scenario catalogue with measured data, not unit tests. Open: where do we run it (local Docker, or the Oracle target) and which scenarios first.

## Tasks
| ID | Spec | Route | Branch | Commit |
|---|---|---|---|---|
| T1 | S1, S6 | inline, test-first (backend order param + cursor, frontend toggle); D1 first | `feat/gallery-order-toggle` | this commit |
| T2 | S2 | design: ADR for metadata + album rules (D2), then migration → own commit, independent verify | `feat/media-metadata` | pending |
| T3 | S2 | derived albums/carousels API + UI, test-first, after T2 | `feat/auto-albums` | pending |
| T4 | S5 | type filter photos/videos (`type` param, index check with EXPLAIN, UI chips), test-first | `feat/gallery-type-filter` | pending |
| T5 | S3 | stress scenario catalogue + scripts under `scripts/stress/` (not part of CI), results and improvements recorded in `docs/runbook.md`; any fix found becomes its own task | `chore/stress-scenarios` | pending |

## Log
- **L3** (T1 evidence, parent): D1 resolved by L2 (order only in Recientes; `type` composes later). Backend RED: `ListOrdered`/`OrderOldest` undefined and `?order=sideways` returned 200; GREEN: `go test ./... && go vet ./...` pass. `EXPLAIN QUERY PLAN` of the ascending keyset query: `SCAN m USING COVERING INDEX media_visible_cursor` (no temp b-tree; the DESC partial index is read backwards, no migration). Frontend RED: 6 failing; GREEN: `npm test` 209 passed, svelte-check 0 errors, build OK, guest JS 35.33 KB gzip (≤ 35.5), CSS 3.77 KB (≤ 4). Behaviour: order change restarts from the first page; late pages from the old order are dropped; in oldest order refresh re-reads all pages only once the guest reached the end. Existing tests touched: two `fetchPage` call assertions now include the order argument; `fakeGallery` implements `ListOrdered`. Docs: `docs/api.md`. Risk: item 3 considered: `GET /api/media` default behaviour is unchanged (new optional param). Real-phone check pending.
- **L1** (user, 2026-10-18): "Vamos a rellenar de nuevo las tareas pendientes. Tengo varias ideas que creo que pueden faltar. Lo primero de todo es algo mas de funcionalidad como la de un filtro de mas recientes a mas antiguas, ademas me gustaria poder llegar a poder jugar con el tema de la metadata ya que se podrian gestionar de forma autonoma algunos albunes o carrusels. Hacer algunos test mas. Despues de anadir estas cosas ayudame a debatir que mas puedo poner para el tema de ordenar"
- **L2** (user, 2026-10-18): "El tema de ordenar de viejo a nuevo o al reves debe estar separado de los gustados. El otro filtro podria ser filtrar por solo fotos o videos. mas test no me refiero de codigo si no ideas o pruebas con datos de rendiiento o estabilidad de la app ante diferentes estres ya sea por concurrencia, por saturacion o mala red quiero ver diferentes casos para ver fallos y que se puede mejorar"
