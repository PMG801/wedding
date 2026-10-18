# Pinned posts (info, games, announcements)
Status: DONE (pending review/merge of `feat/pinned-posts-smoke`). Stacked branches from `develop` (7fad6dd): `feat/pinned-posts-contract` → `-storage` → `-api` → `-ui` → `-smoke`; only the last one is merged.
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
- [x] **T0 [S5, D1-D7] — Contract: ADR-0013, `docs/api.md`, `docs/data-model.md` (migration 0003)** · worker · branch `feat/pinned-posts-contract` · evidence: docs re-read, tables consistent; parent fixes: `position` CHECK `>= 0` with temporary high positions on reorder (UNIQUE is checked per row), `detail_url` nullable in `image`; worker readings kept: all three body fields required, `body` not trimmed, guest route also has `500 pinned_failed`/`503 pinned_unavailable`, `PUT /{id}` keeps position, literal `/order` route wins over `/{id}` · commit: this commit
- [x] **T1 [S1, S3, S6, S7] — Migration `0003_pinned_items` + store with validation** · worker, test-first · risk: item 1 (schema migration) → own commit · branch `feat/pinned-posts-storage` · evidence: RED `go test ./internal/db/` → `user_version = 2; want 3` and missing `pinned_items` table; RED `go test ./internal/pinned/` → build failed (`undefined: New/Store/Item/Input/ErrInvalid`, new package). GREEN: `go test -race -count=1 ./internal/db/` ok and `go test -race -count=1 ./internal/pinned/` ok (13 tests: D7 boundaries 120/121 title, 2000/2001 body, multibyte code points, control chars, media refs, 5th ok / 6th `ErrLimitReached`, rejected writes leave rows unchanged, reorder exact-set, concurrent creates ≤ 5). TRIANGULATE: mutation `< 5` → `<= 5` in `pinned.go` makes `TestCreateAppendsAndRejectsSixthWithoutWriting` and `TestConcurrentCreatesNeverExceedLimit` fail (restored). Upgrade from v2 keeps media and likes rows (`TestMigration0003AddsPinnedItemsToVersionTwoDatabase`). Files: `backend/internal/db/migrations/0003_pinned_items.sql`, `backend/internal/db/db_test.go`, `backend/internal/pinned/pinned.go`, `backend/internal/pinned/pinned_test.go`; also `backend/cmd/boda/main_test.go` schema-version expectation `2 → 3` (T1 hunk). commit: `c89b95b` (≈1100 lines, mostly tests; kept whole as the high-risk unit; `go vet`/`go test -race` green on the commit alone in a scratch worktree)
- [x] **T2 [S1, S2, S6, S7] — Admin CRUD + reorder endpoints and guest read endpoint** · worker, test-first · branch `feat/pinned-posts-api` · evidence: RED `go vet ./internal/httpapi/` → `undefined: WithPinned` (handlers absent). GREEN: `go test -race -count=1 ./internal/httpapi/` ok (15 pinned handler tests: exact bodies and codes, 401 admin/guest, 400 invalid_pinned_item and invalid_pinned_order, 404, 409, 503 unwired, 500 storage failure, 204, `Cache-Control: no-store`, literal `/order` route, `image` null for hidden or thumbnail-less media, `GET /api/media` unchanged by pinned items); `go test -race -count=1 ./cmd/boda/` ok (`TestAppHandlerWiresPinnedItemsForGuestsAndAdmins`). Checks: `cd backend && gofmt -l .` clean; `go vet ./...` ok; `go test -race -count=1 ./...` all 12 packages ok. Files: `backend/internal/httpapi/pinned.go`, `backend/internal/httpapi/pinned_test.go`, `backend/internal/httpapi/health.go` (option field and registration), `backend/cmd/boda/main.go` (`newAppHandler` takes `paths`, `WithPinned`), `backend/cmd/boda/main_test.go` (call sites and wiring test). commit: this commit
- [x] **T3 [S4, S3] — Frontend collapsible «Fijados» section and admin editor** · worker, test-first for API client and pure logic · branch `feat/pinned-posts-ui` · evidence: RED observed for `lib/api/pinned.test.ts` and `lib/pinned/*.test.ts` (unresolved module imports before implementation); GREEN: `npx vitest run src/lib/api/pinned.test.ts` 24 passed, `npx vitest run src/lib/pinned` 17 passed; `cd frontend && npm test` 19 files / 172 tests passed; `npm run build` built; `npx svelte-check` 0 errors, 0 warnings ; parent: media id hint now explains copying the id from `#/foto/<id>` (no admin picker endpoint; follow-up) · commit: this commit
- [x] **T4 [S5] — Runbook and smoke** · inline · branch `feat/pinned-posts-smoke` · evidence: runbook section «Publicaciones fijadas («Fijados»)»; `scripts/smoke.sh` pinned section: 401 without guest/admin session, create with image (after uploading a thumbnail for the smoke photo), guest sees it, hiding the photo drops only the image, 5 created / sixth `409 pinned_limit_reached` with the list unchanged, reorder reverses the guest order, delete `204` then `404`; full smoke exit 0. A first rerun failed in the likes section because another session's smoke used the same Docker stack at the same time; passed once alone · commit: this commit

## Verification
- Independent `gentle-ai-verify` on `c89b95b` (Risk item 1): S1 PASS (store level), S3 PASS, S6 PASS (guarded `INSERT…SELECT`, concurrent test + 25 repeat runs, mixed create/reorder/delete probe), S7 PASS, D7 PASS (store limits); migration matches `docs/data-model.md` and upgrades v2 with rows; `gofmt`, `go vet`, `go test -race` green in a scratch worktree. Severe: none. Non-blocking follow-ups:
  - `pinned.go:289` — only Cc control characters are rejected; zero-width space and U+2028 are accepted (consistent with D7's wording).
  - `pinned.go:103,156,235` — deferred transactions; heavy contention could surface `SQLITE_BUSY` as `500 pinned_failed` (never a cap breach).
  - `pinned.go:173-181` — Update tells not-found from invalid with a second read after a 0-row UPDATE.

## Review
- Native review `review-41f0182e9134c3ba` of `c89b95b` only (range `feat/pinned-posts-contract..feat/pinned-posts-storage`, the high-risk commit; lenses risk, resilience, reliability; 5 files, 1109 lines): **approved**, acknowledged (authority burned). Non-blocking advisories, later work: R1-001 `pinned/pinned.go:111-124` (guarded create statement), R4-001 `pinned/pinned.go:155-175` (update transaction under contention).
- T2/T3 had no native review: not high-risk; covered by tests, the smoke and the T1 verifier.

## Known limits
- No admin picker for the image: the admin copies the photo id from `#/foto/<id>` (hint in the editor and runbook). A picker needs a contract addition.
- Commit sizes exceed the ~400-line review target: `c89b95b` ≈1100 (mostly tests, kept whole as the high-risk unit), `5dbe20c` ≈870 and `03268f5` ≈1360 (each one behaviour with its tests). Review each branch of the stack separately.

## Log
- **L1** (user, earlier): order confirmed admin-moderation → gallery-likes → gallery-windowing → pinned-posts.
- **L2** (user, 2026-10-18), answering the proposal "Qué se fija: entradas de texto del admin (título, cuerpo y una imagen opcional) · «Fijados»: abierta hasta que el invitado la cierra; luego plegada en su dispositivo · Cuántas: máximo 5, en el orden que elija el admin · Formato: solo texto plano, sin HTML ni Markdown" and "¿Mergeo antes feat/gallery-windowing a develop?": "me parece correcto"
- **L3** `feat/gallery-windowing` fast-forwarded into local `develop` (7fad6dd), branch deleted; not pushed (no explicit push instruction this time).
