# Pinned posts (info, games, announcements)
Status: PLANNED (not started). Branch when started: `feat/pinned-posts` from `develop`.
Tests: `cd backend && go test ./... && go vet ./...`; `cd frontend && npm test && npm run build`; `scripts/smoke.sh`.

## Objective
Let the admin pin content (event info, a game, announcements) at the top of the gallery, optionally inside a collapsible section so it does not always occupy the view.

## Open decisions (scope risk: this is a new content type)
- **D1 — What is pinned.** (a) pin an existing photo/video, or (b) a text/announcement entry written by the admin (title + body, optional image). Recommendation: (b) as its own `pinned_items` table, since a game or info is text; pinning existing media can be added later. Keeps media model untouched.
- **D2 — Collapsible.** Single "Fijados" section, collapsed/expanded state remembered per device in `localStorage`; default expanded with unread marker? Recommendation: expanded until first dismissed, then collapsed.
- **D3 — Quantity.** Cap (e.g. 5) and manual order set by admin.
- **D4 — Format.** Plain text only (no HTML/Markdown) to avoid XSS surface.

## Specs (draft)
- **S1** Admin creates, edits, reorders and deletes pinned items (admin session required, `401 admin_session_required` otherwise).
- **S2** Guests read pinned items from an endpoint behind the guest session; they are never mixed into the paginated media cursor.
- **S3** Text rendered as plain text, length-limited and validated server-side.
- **S4** Gallery shows the collapsible "Fijados" section above the grid; Spanish copy.
- **S5** Docs and ADR (Spanish).

## Tasks
- [ ] **T0 [D1-D4] — Decide and write ADR** · user decision + inline · commit: pending
- [ ] **T1 [S1, S3] — Migration `pinned_items` + store with validation** · inline, test-first · risk: item 1 (schema migration) → own commit · commit: pending
- [ ] **T2 [S1] — Admin CRUD endpoints** · inline, test-first · commit: pending
- [ ] **T3 [S2] — Guest read endpoint** · inline, test-first · commit: pending
- [ ] **T4 [S4] — Frontend collapsible section and admin editor** · inline · commit: pending
- [ ] **T5 [S5] — Docs and smoke** · inline · commit: pending
