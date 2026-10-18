# Admin moderation: hide and show media
Status: IN PROGRESS. Stacked branches from `develop`, each from the previous one; only the last branch is merged (L2): `feat/admin-moderation-storage` (T0-T1) → `feat/admin-moderation-api` (T2-T3) → `feat/admin-moderation-ui` (T4-T5).
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`. Sources of truth: `docs/data-model.md` (state machine), ADR-0007, ADR-0009, `docs/api.md`.

## Specs
- **S1** Hide moves the original to `hidden/` and its published derivatives back to private `tmp/<id>_<t|d>.staged`, then sets `media.visibility = 'hidden'`. Show reverses it: `visibility = 'visible'`, original back to `originals/`, staged derivatives republished. The file never stays served while SQLite says `hidden`; a failed step rolls back what it already moved. Reconciliation makes the disk location follow SQLite ("el disco decide si un archivo existe; SQLite decide si es visible").
- **S2** `PATCH /api/admin/media/{id}` with body exactly `{"visibility":"hidden"|"visible"}` (D1). Idempotent: hiding a hidden item (or showing a visible one) returns success without changes. Unknown id gives `404`; invalid body gives `400`; no admin session gives `401 admin_session_required`.
- **S3** Hidden media: gallery excludes it, `/m/*` returns `404`, and reconciliation/`reindex` never quarantine or restore it wrongly.
- **S4** Admin UI: hide/show action in the detail view and a "Ocultas" list; Spanish copy. No delete action anywhere.
- **S5** Docs: complete the `PATCH` contract in `docs/api.md` and the runbook (Spanish).
- **S6** No deletion (D3, user L2): "no quiero que se borre nada desde la ui aunque sea admin, para eso esta el tema de ocultar contenido, nada de borrar". No endpoint, store method or UI control deletes media.

## Decisions
- **D1 — Request body: RESOLVED (L2).** `{"visibility":"hidden"|"visible"}`, no extra fields.
- **D2 — Hidden listing: RESOLVED (L2).** Separate admin route `GET /api/admin/media?visibility=hidden`, same cursor as the gallery.
- **D3 — Delete: RESOLVED (L2).** Never, not even for the admin; see S6.
- **D4 — Hidden derivatives (technical).** They go back to `tmp/*.staged`, the private state reconciliation already keeps for hidden originals (`docs/runbook.md`), so `hidden/` stays originals-only for delivery (ADR-0010). The admin sees hidden previews through an app route behind the admin session, never through Caddy.

## Tasks
- [ ] **T0 [S2, S6, D1-D4] — Record decisions and fix the API contract in `docs/api.md`** · inline · branch `feat/admin-moderation-storage` · commit: pending
- [ ] **T1 [S1, S3] — Storage move and store visibility transition, crash-safe** · inline, test-first · risk: item 1 (moves stored data) → own commit · commit: pending
- [ ] **T2 [S2] — `PATCH /api/admin/media/{id}`** · inline, test-first · branch `feat/admin-moderation-api` · commit: pending
- [ ] **T3 [S2, S3] — Admin hidden listing, admin preview route, reconciliation with `hidden/`** · inline, test-first · commit: pending
- [ ] **T4 [S4, S6] — Admin UI** · inline · branch `feat/admin-moderation-ui` · commit: pending
- [ ] **T5 [S5] — Docs and smoke** · inline · commit: pending

## Log
- **L1** (planning, commit `603f814`): "From the admin session, hide (and restore) specific photos/videos so guests no longer see them."
- **L2** (user, 2026-10-18): "1 no quiero bloquear el trabajo por lo que mejor el hacer las ramas que salgan una de tras de otra y luego solo mergeo la ultima rama pero de esta forma puedo ver mas claramente y hacer ciertos cambios. 2 me parece corerto el primer orden. 3 las 2 priemaras bien pero la ultima se quita no quiero que se borre nada desde la ui aunque sea admin, para eso esta el tema de ocultar contenido, nada de borrar"
- **L3** Order confirmed: admin-moderation → gallery-likes → gallery-windowing → pinned-posts; gallery-search-and-ordering stays a discussion.
