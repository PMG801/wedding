# Pinned posts (info, games, announcements)
Status: IN PROGRESS (authorized L2). Stacked branches from `develop` (7fad6dd): `feat/pinned-posts-contract` → `-storage` → `-api` → `-ui` → `-smoke`; only the last one is merged.
Tests: `cd backend && gofmt -l . && go vet ./... && go test -race ./...`; `cd frontend && npm test && npm run build && npx svelte-check`; `scripts/smoke.sh`.

## Objective
Let the admin pin content (event info, a game, announcements) at the top of the gallery, inside a collapsible section so it does not always occupy the view.

## Decisions (D1-D4 accepted by the user, L2; D5-D7 technical, derived by the parent)
- **D1 — What is pinned.** A text entry written by the admin: title + body + optional image, in its own `pinned_items` table. The media model stays untouched.
- **D2 — Collapsible.** One «Fijados» section above the gallery views. Expanded until the guest collapses it; the collapsed/expanded choice is remembered per device in `localStorage` (key `boda.pinned.collapsed`).
- **D3 — Quantity.** At most 5 pinned items, in a manual order set by the admin.
- **D4 — Format.** Plain text only (no HTML, no Markdown); rendered as text with line breaks preserved.
- **D5 — Optional image = an existing visible medium.** The admin picks a photo already uploaded to the gallery by its id (`media_id`); there is no new upload path. The guest endpoint only returns the image while that medium is visible and has a thumbnail; a hidden medium makes the entry show without image (the entry is not removed).
- **D6 — Deleting pinned items is allowed.** The "no delete" rule of ADR moderation applies to guest media; a pinned item is admin-authored text, so the admin can delete it (S1, accepted in L2).
- **D7 — Contract.** Limits: title 1-120 characters after trimming, body 0-2000 characters, both counted in Unicode code points, no control characters except `\n` in the body; JSON body ≤ 8 KiB, unknown fields rejected. Error codes: `400 invalid_pinned_item`, `401 admin_session_required`, `404 pinned_not_found`, `409 pinned_limit_reached`, `400 invalid_pinned_order` (ids must be exactly the current set). Guest endpoint behind the guest session, `401 guest session required` text like the other guest routes. Exact endpoints and envelopes in `docs/api.md` (T0).

## Specs
- **S1** Admin creates, edits, reorders and deletes pinned items (admin session required, `401 admin_session_required` otherwise).
- **S2** Guests read pinned items from an endpoint behind the guest session; they are never mixed into the paginated media cursor.
- **S3** Text rendered as plain text, length-limited and validated server-side.
- **S4** Gallery shows the collapsible "Fijados" section above the grid; Spanish copy.
- **S5** Docs and ADR (Spanish).
- **S6** At most 5 pinned items (D3): creating a sixth returns `409 pinned_limit_reached` and stores nothing.
- **S7** Optional image (D5): only an existing confirmed medium can be referenced; while it is hidden the guest sees the entry without image.

## Tasks
- [ ] **T0 [S5, D1-D7] — Contract: ADR-0013, `docs/api.md`, `docs/data-model.md` (migration 0003)** · worker · branch `feat/pinned-posts-contract` · commit: pending
- [ ] **T1 [S1, S3, S6, S7] — Migration `0003_pinned_items` + store with validation** · worker, test-first · risk: item 1 (schema migration) → own commit · branch `feat/pinned-posts-storage` · commit: pending
- [ ] **T2 [S1, S2, S6, S7] — Admin CRUD + reorder endpoints and guest read endpoint** · worker, test-first · branch `feat/pinned-posts-api` · commit: pending
- [ ] **T3 [S4, S3] — Frontend collapsible «Fijados» section and admin editor** · worker, test-first for API client and pure logic · branch `feat/pinned-posts-ui` · commit: pending
- [ ] **T4 [S5] — Runbook and smoke** · inline · branch `feat/pinned-posts-smoke` · commit: pending

## Log
- **L1** (user, earlier): order confirmed admin-moderation → gallery-likes → gallery-windowing → pinned-posts.
- **L2** (user, 2026-10-18), answering the proposal "Qué se fija: entradas de texto del admin (título, cuerpo y una imagen opcional) · «Fijados»: abierta hasta que el invitado la cierra; luego plegada en su dispositivo · Cuántas: máximo 5, en el orden que elija el admin · Formato: solo texto plano, sin HTML ni Markdown" and "¿Mergeo antes feat/gallery-windowing a develop?": "me parece correcto"
- **L3** `feat/gallery-windowing` fast-forwarded into local `develop` (7fad6dd), branch deleted; not pushed (no explicit push instruction this time).
