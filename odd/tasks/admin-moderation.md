# Admin moderation: hide and show media
Status: PLANNED (not started). Branch when started: `feat/admin-moderation` from `develop`.
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`.

## Objective
From the admin session, hide (and restore) specific photos/videos so guests no longer see them.

## Context
Already designed but unimplemented: `media.visibility` (`visible`/`hidden`) with partial indexes, a non-served `hidden/` directory, and `PATCH /api/admin/media/{id}` whose body and codes are pending in `docs/api.md`. Hiding moves the file to `hidden/` and updates SQLite; the URL gives 404 immediately; admin can see hidden items. Source of truth: `docs/data-model.md` (state machine), ADR-0009 (reconciliation must understand `hidden/`).

## Open decisions
- **D1 — Request body.** Recommendation: `{"visibility":"hidden"|"visible"}`, no extra fields.
- **D2 — Hidden listing.** `GET /api/admin/media?visibility=hidden` with the same cursor, vs. a flag on the guest route. Recommendation: separate admin route.
- **D3 — Delete.** Out of scope; hide is reversible, delete is not.

## Specs (draft)
- **S1** Hide: move original and derivatives to `hidden/`, then update SQLite; order and crash-safety follow the "disk decides existence, SQLite decides visibility" rule. Show: reverse.
- **S2** Idempotent: hiding a hidden item (or showing a visible one) returns success without changes. Unknown id gives `404`; invalid body gives `400`; no admin session gives `401 admin_session_required`.
- **S3** Hidden media: gallery excludes it, `/m/*` returns `404`, and reconciliation/`reindex` never quarantine or restore it wrongly.
- **S4** Admin UI: hide/show action in the detail view and a "Ocultas" list; Spanish copy.
- **S5** Docs: complete the `PATCH` contract in `docs/api.md` and the runbook (Spanish).

## Tasks
- [ ] **T0 [D1-D2] — Fix the API contract in `docs/api.md`** · inline · commit: pending
- [ ] **T1 [S1, S3] — Storage move and store visibility transition, crash-safe** · inline, test-first · risk: item 1 (moves stored data) → own commit · commit: pending
- [ ] **T2 [S2] — `PATCH /api/admin/media/{id}`** · inline, test-first · commit: pending
- [ ] **T3 [S2, S3] — Admin hidden listing + reconciliation test with `hidden/`** · inline, test-first · commit: pending
- [ ] **T4 [S4] — Admin UI** · inline · commit: pending
- [ ] **T5 [S5] — Docs and smoke** · inline · commit: pending
