# Photo upload and wedding gallery
Branch: `feat/photo-gallery-api` from `develop`; delivery: stacked review slices toward `develop`.
Tests: `cd backend && go test ./... && go vet ./...`; frontend `cd frontend && npm test && npm run build`.

## Objective
Deliver the first usable guest flow: enter through the wedding QR link, upload photos safely from a phone, and browse the resulting images in a polished wedding-themed frontend.

## Problem and rationale
The repository currently has a health-check-only API and frontend. The accepted architecture already defines local-disk media storage, QR guest access, client-generated photo derivatives, streaming/idempotent uploads, and a gallery; this feature implements that first vertical slice without introducing a new service or media-processing dependency.

## Accepted scope and constraints
- Photos only for this iteration: JPEG, PNG, HEIC, and HEIF. Defer video; its 1.5 GB limit, slower/single-upload behavior, poster generation, and Range playback make it a separate work unit rather than a simple extension.
- Follow `docs/architecture.md`, `docs/runbook.md`, and ADRs `docs/adr/0002-almacenamiento-medios-disco-local.md`, `0004-miniaturas-cliente-respaldo-diferido.md`, `0005-subida-streaming-idempotente.md`, `0006-serving-estaticos-proxy-caddy.md`, `0007-acceso-qr-seguridad.md`, `0008-limites-operativos-control-disco.md`, and `0009-limpieza-conciliacion-huerfanos.md`.
- Guest access is QR-token-based; preserve the token-to-signed-cookie redirect and keep hidden media inaccessible. Do not add guest accounts, a public unauthenticated upload mode, admin moderation, server-side transcoding, or cloud storage.
- Generate small JPEG derivatives in the browser when possible; retain the documented original-only fallback when decoding/derivative generation fails.
- Delivery plan: stacked review slices toward `develop` (selected by the user). Local work-unit commits are authorized. The user explicitly authorized local branch `feat/photo-gallery-api` from `develop` for T3. Push, PR creation, and merge remain unauthorized.
- Keep each cohesive review slice near or under 400 changed lines; record the actual line count and dependencies as work is completed.
- Engram mirror topic: `odd/photo-upload-gallery/tasks`.

## Specs
- **S1 — User-approved T3 contract (2026-10-14).** The user selected “Sí, aplica la propuesta” for this exact proposal:
  > - Derivados: `POST /api/media/{id}/derived?variant=t|d`, un JPEG crudo por petición, con sesión de invitado, UUID canónico y límites actuales de tamaño/concurrencia. Permitir subirlos antes del original en almacenamiento privado; respuestas idempotentes y no reescritura.
  > - Galería: `GET /api/media?cursor=...&limit=50`, más recientes primero, cursor estable y opaco; sólo originales confirmados y visibles. URLs de derivados opcionales si aún faltan.
  > - Publicación: Caddy sirve sólo medios publicados según ADR-0006; los ocultos responden 404; sin transcodificación en servidor.
  > - Conciliación: borrar parciales obsoletos; poner originales sin registro en cuarentena, nunca borrarlos; proteger subidas activas.
- **S2 — Existing feature scope and acceptance criteria.** Preserve the accepted boundaries and apply acceptance criteria 1–6 below; video, cloud storage, server-side transcoding, guest accounts, and moderation remain out of scope.
- **S3 — Approved gallery contract (2026-10-15, see L8).** `GET /api/media?cursor=<opaque>&limit=50` requires the guest session; `limit` 1–100, default 50; invalid cursor → `400 {"error":"invalid_cursor"}`. Cursor is base64url of `confirmed_at:id` using keyset pagination on `media_visible_cursor`. Response `{"items":[{id, mime, confirmed_at, original_url, thumb_url|null, detail_url|null}], "next_cursor": string|null}` with `Cache-Control: no-store`. Derivative presence is checked on disk (no new column). HEIC without thumbnail renders a placeholder plus download, never the original in `<img>`. Caddy serves `/m/o/*` and `/m/d/*` per ADR-0006 (`immutable`, `nosniff`, no compression, `?dl=1` attachment, explicit `.heic` type).
- **S4 — Access restricted by QR key (2026-10-15, user, verbatim):** “El acceso a la aplicación (tanto para subir como para ver) debe quedar restringido y condicionado a la clave/token implementada en el código QR. Nadie que no escanee o tenga el enlace con esa key debería poder interactuar con la galería ni subir contenido.”
- **S5 — UI language (2026-10-15, user, verbatim):** “La interfaz de usuario debe estar completamente en español, manteniendo rigurosamente todo el código interno, identificadores y variables en inglés.”
- **S6 — Frontend shape (approved 2026-10-15).** Own hash router (`#/`, `#/foto/{id}`), no dependency; one mobile-first guest screen: upload button, per-file queue, infinite gallery; detail in `<dialog>` with swipe/keyboard and original download. Upload queue is pure TypeScript with injectable `fetch`/clock, tested with Vitest (2 concurrent, 5 retries with backoff, original-only fallback). Sober wedding style (ivory, sage accent, self-hosted serif headings); welcome text static until admin settings exist. No Playwright yet; extend `scripts/smoke.sh`. Admin moderation is a separate later feature; video stays deferred. Wedding is in late February: domain and device tests must close before QR printing.

## Tasks
- [x] **T1 — Secure guest QR session**
  - Route: one bounded `gentle-ai-worker` implementation; parent coordinates and records evidence.
  - Scope: implement `/e/{token}` validation, signed guest cookie, protected-handler seam, server dependency wiring, and safe local/runbook token provisioning; include focused tests and Spanish runbook documentation.
  - Files: `backend/internal/auth/**`, `backend/internal/httpapi/**`, `backend/internal/config/**`, `backend/cmd/boda/**`, `backend/**_test.go`, and `docs/runbook.md`.
  - Checks: focused auth/HTTP tests; `cd backend && go test ./... && go vet ./...`.
  - Evidence: TDD RED observed with `cd backend && go test ./internal/auth ./internal/httpapi`; targeted GREEN and triangulation passed with `go test ./internal/auth ./internal/httpapi` and `go test ./internal/auth ./internal/httpapi ./cmd/boda`. Independent verification passed `cd backend && go test ./... && go vet ./...` and `git diff --check`. Behavior diff: 273 changed lines; full T1 commit including task tracking: 344 changed lines, within the review budget. Secure-cookie local use requires HTTPS; no insecure exception was added.
  - Commit: `00fc950` (`feat(auth): add secure QR guest sessions`).
- [x] **T2a — Build the photo persistence core**
  - Route: one bounded `gentle-ai-worker`; tests stay with the core behavior.
  - Scope: add the independently testable photo validation/streaming/finalization service and SQLite media repository. Use canonical client-generated UUIDv4 IDs; check JPEG/PNG/HEIC/HEIF signatures and declared size, enforce disk and core-concurrency limits, stream to same-filesystem `.part` files, sync and atomically confirm, and avoid rewriting an already-confirmed ID. Do not expose an HTTP route in this slice; HTTP auth, fail-fast 503 + `Retry-After` on saturation, request-idle handling, and status mapping belong to T2b.
  - Files: `backend/internal/upload/**`, `backend/internal/media/**`, `backend/internal/db/**`, `backend/internal/storage/**`, and related Go tests.
  - Checks: focused validation/storage/database tests; `cd backend && go test ./... && go vet ./...`.
  - Evidence: initial combined T2 forecast was 550–700 changed lines; no files were changed and it was split before implementation. Core TDD RED observed (`go test ./internal/upload` failed before `DetectPhotoType` existed); GREEN passed for signature, UUID, size, cleanup, disk, persistence, and idempotency tests. Independent verification passed `cd backend && go test ./... && go vet ./...` and `git diff --check`. Core diff was exactly 400 additions. Review confirmed the core semaphore bounds active work; the documented fail-fast overload response (503 + `Retry-After`) is assigned to T2b. Blocked-reader cancellation is an HTTP-body concern for T2b; no other T2a blocker was found. Native start for this exact candidate was declined; no lineage was created and no reviewers ran. ASSESS with the exact candidate outcome `declined` returned risk `medium`, self-verification required, no separate verifier required; `reviewDue=true` (`slice_budget_reached`). Native continuation, relayed verbatim: `gentle-ai review status --cwd=/home/pau/Projects/wedding --contract=gentle-ai.review-integration/v2 --next-transition=true --base-ref=a432275d9a9078602f7bc946245eecab111a76ab --committed-only=true`.
  - Commit: `8977132` (`feat(upload): add bounded photo persistence core`).
- [x] **T2b — Expose authenticated, idempotent photo uploads**
  - Route: one bounded `gentle-ai-worker`; HTTP tests and Spanish API contract stay with the route.
  - Scope: wire the T2a core into authenticated `PUT /api/media/{id}` with a canonical UUIDv4 path ID and raw request body, reject saturated capacity without blocking using 503 + `Retry-After`, enforce the 90-second idle read limit and `Content-Length`, return 201 for new confirmation and 200 for a confirmed retry without rewriting, map the minimum-free-disk rejection to an explicit insufficient-storage response, and document the request/response/error contract.
  - Files: `backend/internal/httpapi/**`, `backend/cmd/boda/**`, related Go tests, and `docs/api.md`.
  - Checks: focused guest-authenticated HTTP tests; `cd backend && go test ./... && go vet ./...`.
  - Evidence: TDD RED observed for the initial route and for the low-disk response (expected 507, observed 500); both GREEN after implementation. Independent verification passed `cd backend && go test ./... && go vet ./...` and `git diff --check`; complete T2b behavior delta is 397 changed lines. Guest auth, raw-body validation, idempotent 201/200, prompt 503 + `Retry-After`, and low-disk 507 are covered. The 90-second deadline is structurally verified, but a live slow-client timeout was not exercised; that check is isolated in T2c. Native consent for this exact candidate was declined (high risk); no lineage/reviewers. Candidate-bound ASSESS reports high risk, self-verification and independent verification required, `reviewDue=true` (`high_risk`). Native continuation, relayed verbatim: `gentle-ai review status --cwd=/home/pau/Projects/wedding --contract=gentle-ai.review-integration/v2 --next-transition=true --base-ref=5b1fa8b0c7001fc898fd340a37fff8655976a0ff --committed-only=true`.
  - Commit: `ec7f958` (`feat(upload): expose secure idempotent photo upload API`).
- [x] **T2c — Verify idle upload timeout over HTTP**
  - Route: one bounded `gentle-ai-worker`; keep the local slow-client test, error mapping, and Spanish API contract together.
  - Scope: exercise the body-idle deadline with a local slow client and a short test-only timeout; map an expired upload to a stable 408 `upload_idle_timeout` response if necessary, then document it.
  - Files: `backend/internal/httpapi/photo_upload.go`, `backend/internal/httpapi/upload_test.go`, and `docs/api.md`.
  - Checks: focused live HTTP idle-timeout test; `cd backend && go test ./... && go vet ./...`.
  - Evidence: a local TCP slow-client test sends a partial body against a short test-only timeout and observed the stable `408 upload_idle_timeout` response; a complete request to the same server still returns `201`. Independent verification (Terra-model exception because the configured Sol verifier was unavailable) passed `cd backend && go test ./internal/httpapi -run '^TestPhotoUploadIdleTimeoutOverHTTP$' -count=1`, `cd backend && go test ./... && go vet ./...`, and `git diff --check ec7f958..11b41a3`. The initial sandbox cannot bind loopback sockets; the HTTP checks passed when rerun with permitted local networking. Candidate assessment against `ec7f958` is medium risk but `under_budget` (103 changed lines), so no native review preflight is due.
  - Commit: `11b41a3` (`feat(upload): implement idle timeout handling for photo uploads`).
- [x] **T3a [S1] — Persist client JPEG derivatives privately**
  - Route: inline parent implementation; sequential bounded work, no Writer-rule trigger. This changes persisted media and storage boundaries; follow the native ASSESS verification plan.
  - Scope: stream one `t`/`d` JPEG derivative to private staging before its original exists, validate its signature and size, enforce disk/concurrency limits, refuse rewrites, and promote staged variants only after the original is confirmed visible.
  - Files: `backend/internal/upload/**`, `backend/internal/storage/**`, and related Go tests.
  - Checks: `cd backend && go test ./internal/upload ./internal/storage`; `cd backend && go test ./... && go vet ./...`; `git diff --check`.
  - Evidence: TDD RED observed because `Store.SaveDerivative` did not exist. The implementation streams `t`/`d` JPEGs into private staging, validates signature and declared size, applies the configured byte, free-disk, and concurrency limits, refuses rewrites, and promotes only after a visible original is confirmed. Tests cover private staging and promotion, hidden originals, invalid signatures and body sizes, size and disk limits, concurrency, and staged retry without rewrite. `cd backend && go test ./internal/upload ./internal/storage`, `cd backend && go test ./... && go vet ./...`, and `git diff --check` pass. The implementation slice was 401 changed lines including task tracking (377 insertions, 24 deletions); the additional coverage and task-record follow-up are kept as a separate bounded commit. A transient promotion failure can leave a private staged file that an ordinary retry does not reconcile; retry/recovery remains T3d. T3a has no remaining blocker.
  - Commits: `db6ea3f` (`feat(upload): persist private JPEG derivatives`); `f109e14` (`test(upload): complete derivative persistence coverage`).
- [x] **T3b [S1] — Expose authenticated derivative uploads**
  - Route: inline parent implementation after T3a; sequential bounded work, no Writer-rule trigger.
  - Scope: expose `POST /api/media/{id}/derived?variant=t|d` as one raw `image/jpeg` request per variant, use the guest session, canonical UUID and existing limits, share the original-upload concurrency gate, return idempotent 200/201 responses, and document all outcomes in Spanish.
  - Files: `backend/internal/httpapi/**`, related Go tests, and `docs/api.md`.
  - Checks: focused derivative HTTP tests; `cd backend && go test ./... && go vet ./...`; `git diff --check`.
  - Evidence: RED observed when the route returned 404 instead of guest-auth 401 and when `application/octet-stream` was accepted with 201 instead of 415. GREEN: `cd backend && go test ./... && go vet ./...` and `git diff --check` pass (2026-10-15). The original and derivative routes share one fail-fast capacity gate. Behavior diff: 295 insertions, 11 deletions.
  - Commit: `feat(upload): expose authenticated derivative uploads` (hash recorded in the next task commit).
- [x] **T3c [S1, S3] — List visible gallery media**
  - Route: inline parent implementation; no Writer trigger (one sequential unit).
  - Scope: `internal/media.Gallery` with keyset pagination over `media_visible_cursor`, opaque base64url `confirmed_at:id` cursor, derivative URLs from published files on disk (`upload.DerivativeFileName` shared with the store), and guest-authenticated `GET /api/media` with `limit` 1–100, `no-store`, `invalid_limit`/`invalid_cursor` 400s; Spanish contract in `docs/api.md`.
  - Evidence: RED observed (`go test ./internal/media ./internal/httpapi` failed to build: `Gallery`, `NewGallery`, `Page`, `ErrInvalidCursor` undefined). GREEN: `cd backend && go test ./... && go vet ./...`, `gofmt -l` clean, `git diff --check` pass. Tests cover newest-first order with tied timestamps, cursor stability after a newer upload, hidden exclusion, published-vs-staged derivative URLs, empty page JSON, malformed cursors, limit bounds, guest 401, and 503 without a lister. About 530 changed lines (roughly half tests): above the 400-line heuristic but one cohesive behavior; not split artificially.
  - Commit: `feat(gallery): list visible media with stable cursor pagination` (hash recorded in the next task commit). T3b commit: `c28d0cb`.
- [x] **T3e [S3, S4] — Serve published media behind the guest session**
  - Route: inline parent implementation.
  - Scope: Caddy `/m/o/*` and `/m/d/*` from read-only mounts per ADR-0006 (no compression, `nosniff`, `?dl=1` attachment, explicit `.heic` type), gated by `forward_auth` to a guest-session check endpoint so S4 holds for files too; new ADR recording the change to ADR-0006; extend `scripts/smoke.sh` (gallery listing, authorized file fetch, unauthenticated 401).
  - Risk: item 2 (access guard for media files) and item 5 (Compose volumes, Dockerfile, Caddy deploy config) — independent verifier for this unit.
  - Evidence: RED `go test ./internal/httpapi -run TestGuestSessionCheck` failed before `/api/session` existed; GREEN after. End-to-end RED: the extended `scripts/smoke.sh` failed with “Unauthenticated media file returned HTTP 200; want 401” (SPA fallback). GREEN: `scripts/smoke.sh` passes health, QR entry, upload/retry, gallery 401/listing, media 401 without cookie, byte-identical original, `private, max-age=31536000, immutable`, `nosniff`, no `Content-Encoding`, `?dl=1` attachment, 404 for missing/hidden/traversal paths, Range `206`, and `image/heic` type. `caddy validate` passes (caddy:2-alpine). Found and fixed: Compose had no persistent shared volume, so Caddy could not see media; added the `event-data` volume (app at `/srv/evento`, web subpaths `originals`/`derived` read-only) and a nonroot-owned `/srv/evento` in the app image. ADR-0011 records the partial supersession of ADR-0006. `cd backend && go test ./... && go vet ./...` and `git diff --check` pass.
  - Commit: `feat(media): serve published media behind the guest session` (hash recorded in the next task commit). T3c commit: `ae1e64b`.
- [ ] **T3d [S1] — Reconcile stale upload artifacts safely** (pending)
  - Route: inline parent implementation; sequential bounded work, no Writer-rule trigger.
  - Scope: run reconciliation at the configured interval, remove stale partial files only when no upload is active, quarantine unindexed originals without deleting them, and retry promotion of staged derivatives only when their original is confirmed visible; preserve hidden/unconfirmed media and never delete confirmed media.
  - Files: `backend/internal/jobs/**`, `backend/internal/media/**`, `backend/internal/storage/**`, `backend/internal/upload/**`, `backend/cmd/boda/**`, related Go tests, and relevant Spanish runbook/API docs.
  - Checks: focused reconciliation/active-upload tests; `cd backend && go test ./... && go vet ./...`; `git diff --check`.
  - Evidence: pending.
  - Commit: pending.
- [ ] **T4 — Build the browser photo upload queue**
  - Route: inline by default; re-evaluate Writer-rule triggers when the task starts. Keep deterministic tests with the queue/API behavior.
  - Scope: photo selection, browser-side thumbnail/detail generation, two-photo concurrency, upload order, per-file progress/status, bounded retries, and original-only fallback when a derivative cannot be made.
  - Files: `frontend/src/lib/upload/**`, `frontend/src/lib/media/**`, `frontend/src/lib/api/**`, and their tests.
  - Checks: `cd frontend && npm test && npm run build`.
  - Evidence: pending.
  - Commit: pending.
- [ ] **T5 — Create the wedding photo gallery experience**
  - Route: inline by default; re-evaluate Writer-rule triggers when the task starts. Keep visual behavior and responsive states together.
  - Scope: replace the health-only screen with a responsive, wedding-appropriate guest experience for upload and browsing confirmed photos, including loading/empty/error states and accessible image detail viewing.
  - Files: `frontend/src/App.svelte`, `frontend/src/app.css`, `frontend/src/components/**`, `frontend/src/views/**`, and relevant frontend tests.
  - Checks: `cd frontend && npm test && npm run build`; run an end-to-end/browser check if the repository/runtime provides a usable browser harness, otherwise report that limitation explicitly.
  - Evidence: pending.
  - Commit: pending.

## Acceptance criteria
1. Guests must enter through the configured QR token; successful entry removes the token from the visible URL and establishes the documented signed cookie.
2. Authenticated guests can upload supported photos without buffering the original in memory; server-side checks enforce file signature, configured size, free-disk, and concurrency limits.
3. Confirmed retries do not duplicate/rewrite media; failed or interrupted work does not expose partial or hidden media.
4. The browser can create JPEG derivatives when supported, show per-file progress/errors, and recover through the documented original-only fallback.
5. The guest gallery displays only visible confirmed images, supports image detail viewing, and presents a responsive wedding-themed design.
6. Backend tests/vet and frontend tests/build pass; all task evidence and local commit identities are recorded. Video upload/playback is explicitly deferred.

## Progress and verification
- Exploration confirmed that only health endpoints/UI exist; media/auth/jobs packages are placeholders. The existing configuration already defines the QR token, session key, upload limits, and storage paths.
- User decisions: photos first; video deferred because it is not a small extension; authorize local work-unit commits; select stacked review slices toward `develop`. The user authorized local branch `feat/photo-gallery-api` from `develop`; push, PR, and merge remain unauthorized.
- Review workload forecast: the full vertical slice spans backend, frontend, proxy, and documentation and is expected to exceed 400 changed lines; use the seven bounded deliverables and record actual slice size before each commit. T2's initial forecast was 550–700 lines; it was divided into persistence-core and HTTP-integration slices before any T2 edits, with the idle-timeout test kept as a separate small slice.
- Progress: T1 is complete and committed as `00fc950`; T2a as `8977132`; T2b as `ec7f958`; and T2c as `11b41a3`. T2c's live slow-client test, backend suite/vet, and diff checks passed; its medium-risk candidate was under the review slice budget. The latest integrated baseline is `develop` at `8aca776`, which includes the `/e/*` Caddy proxy route and the expanded guest-upload smoke script. The user authorized `feat/photo-gallery-api` from that baseline. The initial T3a draft measured 698 changed lines including the task record, so the derivative core and HTTP API are now separate review slices (T3a–T3b); gallery serving and reconciliation are T3c–T3d. T3a core is in progress. T4 and T5 remain pending.
- Current work-unit base: `8aca776`.

## Next step
Complete T3a (derivative persistence), then T3b (authenticated derivative API), T3c (visible gallery and Caddy serving), and T3d (safe reconciliation); continue with T4 and T5 afterward.

## Log
- L1 — 2026-10-14, resume request (verbatim): “Toca repasar los ultimos cambios donde ya añadimos la api para subir fotos, vamos a repasar las tareas y a continuar.”
- L2 — 2026-10-14: The user approved the displayed T3 contract proposal (“Sí, aplica la propuesta”) and explicitly authorized local branch `feat/photo-gallery-api` from `develop`; push, PR, and merge remain unauthorized.
- L3 — 2026-10-14: Exploration showed T3 combines three independently verifiable work units; split it into T3a–T3c to keep each review slice focused and preserve the existing accepted scope.
- L4 — 2026-10-14: T3a HTTP test-first RED observed: `TestDerivativeUploadRequiresGuestSessionAndStoresRawJPEGIdempotently` got 404 instead of the required guest-session 401 because the route does not yet exist.
- L5 — 2026-10-14: T3a persistence test-first RED observed: `TestSaveDerivativeStaysPrivateUntilPhotoConfirmationAndRetriesWithoutRewrite` failed to build because `Store.SaveDerivative` does not yet exist.
- L6 — 2026-10-14: The initial T3a draft measured 698 changed lines including task tracking; separated derivative persistence from HTTP route/docs, keeping the original accepted scope while reducing review-slice size.
- L7 — 2026-10-14: Independent T3a verification passed without blockers, but requested direct tests for derivative validation, limits, concurrency, and pre-original no-rewrite. A promotion failure can leave staged bytes private; defer retry/reconciliation of those bytes to T3d, gated on visible original confirmation.
- L8 — 2026-10-15, user approval of the recommendations package (verbatim): “¡Aprobado! Vamos adelante con todo el paquete, aplicando los matices que comentas: Seguridad y acceso por QR: Entendido perfectamente. El acceso a la aplicación (tanto para subir como para ver) debe quedar restringido y condicionado a la clave/token implementada en el código QR. Nadie que no escanee o tenga el enlace con esa key debería poder interactuar con la galería ni subir contenido. Idioma de la UI: Autorizado al 100%. La interfaz de usuario debe estar completamente en español, manteniendo rigurosamente todo el código interno, identificadores y variables en inglés. Fecha de la boda: Anotado que es a finales de febrero, así que tenemos margen, pero ten presente cerrar el dominio y las pruebas a tiempo para la impresión del QR. Puedes proceder inmediatamente con el cierre de T3b, hacer el commit y continuar de forma autónoma con T3c, T3d, T4 y T5 según el orden previsto. ¡Dale caña!”
- L9 — 2026-10-15: S4 conflicts with ADR-0006, which serves `/m/*` without authentication (unguessable URLs). ADR-0006 itself names `forward_auth` as the path “si algún día hiciera falta”; S4 is that requirement, so T3e gates media files with the guest session and records a superseding ADR.
- L10 — 2026-10-15: T3c split into the listing API (T3c) and authenticated media serving (T3e) to keep review slices focused.
