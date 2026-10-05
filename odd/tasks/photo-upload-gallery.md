# Photo upload and wedding gallery

## Objective
Deliver the first usable guest flow: enter through the wedding QR link, upload photos safely from a phone, and browse the resulting images in a polished wedding-themed frontend.

## Problem and rationale
The repository currently has a health-check-only API and frontend. The accepted architecture already defines local-disk media storage, QR guest access, client-generated photo derivatives, streaming/idempotent uploads, and a gallery; this feature implements that first vertical slice without introducing a new service or media-processing dependency.

## Accepted scope and constraints
- Photos only for this iteration: JPEG, PNG, HEIC, and HEIF. Defer video; its 1.5 GB limit, slower/single-upload behavior, poster generation, and Range playback make it a separate work unit rather than a simple extension.
- Follow `docs/architecture.md`, `docs/runbook.md`, and ADRs `docs/adr/0002-almacenamiento-medios-disco-local.md`, `0004-miniaturas-cliente-respaldo-diferido.md`, `0005-subida-streaming-idempotente.md`, `0006-serving-estaticos-proxy-caddy.md`, `0007-acceso-qr-seguridad.md`, `0008-limites-operativos-control-disco.md`, and `0009-limpieza-conciliacion-huerfanos.md`.
- Guest access is QR-token-based; preserve the token-to-signed-cookie redirect and keep hidden media inaccessible. Do not add guest accounts, a public unauthenticated upload mode, admin moderation, server-side transcoding, or cloud storage.
- Generate small JPEG derivatives in the browser when possible; retain the documented original-only fallback when decoding/derivative generation fails.
- Delivery plan: stacked review slices toward `develop` (selected by the user). Local work-unit commits are authorized; do not create branches, push, open PRs, or merge without separate authorization.
- Keep each cohesive review slice near or under 400 changed lines; record the actual line count and dependencies as work is completed.
- Engram mirror topic: `odd/photo-upload-gallery/tasks`.

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
- [ ] **T2c — Verify idle upload timeout over HTTP** (in progress)
  - Route: one bounded `gentle-ai-worker`; keep the local slow-client test, error mapping, and Spanish API contract together.
  - Scope: exercise the body-idle deadline with a local slow client and a short test-only timeout; map an expired upload to a stable 408 `upload_idle_timeout` response if necessary, then document it.
  - Files: `backend/internal/httpapi/photo_upload.go`, `backend/internal/httpapi/upload_test.go`, and `docs/api.md`.
  - Checks: focused live HTTP idle-timeout test; `cd backend && go test ./... && go vet ./...`.
  - Evidence: pending.
  - Commit: pending.
- [ ] **T3 — Publish derivatives and a safe guest gallery API**
  - Route: one bounded `gentle-ai-worker`; include relevant API/Caddy/Vite contract docs and tests.
  - Scope: accept client-generated JPEG thumbnail/detail derivatives, list visible gallery items, serve only confirmed visible media through Caddy with the accepted cache/range/security behavior, and reconcile stale partial/orphaned files without deleting unindexed originals.
  - Files: `backend/internal/media/**`, `backend/internal/jobs/**`, `backend/internal/httpapi/**`, `backend/cmd/boda/**`, `backend/**_test.go`, `deploy/caddy/Caddyfile`, `frontend/vite.config.ts`, and `docs/api.md`.
  - Checks: focused gallery/visibility/reconciliation tests; `cd backend && go test ./... && go vet ./...`; inspect the proxy routes for hidden-path isolation.
  - Evidence: pending.
  - Commit: pending.
- [ ] **T4 — Build the browser photo upload queue**
  - Route: one bounded `gentle-ai-worker`; keep deterministic tests with the queue/API behavior.
  - Scope: photo selection, browser-side thumbnail/detail generation, two-photo concurrency, upload order, per-file progress/status, bounded retries, and original-only fallback when a derivative cannot be made.
  - Files: `frontend/src/lib/upload/**`, `frontend/src/lib/media/**`, `frontend/src/lib/api/**`, and their tests.
  - Checks: `cd frontend && npm test && npm run build`.
  - Evidence: pending.
  - Commit: pending.
- [ ] **T5 — Create the wedding photo gallery experience**
  - Route: one bounded `gentle-ai-worker`; visual behavior and responsive states stay together.
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
- User decisions: photos first; video deferred because it is not a small extension; authorize local work-unit commits; select stacked review slices toward `develop`. No branch creation, push, PR, or merge is authorized.
- Review workload forecast: the full vertical slice spans backend, frontend, proxy, and documentation and is expected to exceed 400 changed lines; use the seven bounded deliverables and record actual slice size before each commit. T2's initial forecast was 550–700 lines; it was divided into persistence-core and HTTP-integration slices before any T2 edits, with the idle-timeout test kept as a separate small slice.
- Progress: T1 is complete and committed as `00fc950`. T2a is complete and committed as `8977132`; its native review consent was declined for that exact candidate, no lineage was created, and ASSESS reports review due (`slice_budget_reached`). T2b is complete and committed as `ec7f958` (397 changed lines); independent backend tests/vet passed, but its native consent was declined for that exact candidate and ASSESS reports review due (`high_risk`). The original combined T2 estimate was 550–700 lines and caused no source edits; it was split into T2a and T2b. T2c is in progress because the 90-second timeout has not yet been exercised with a live slow client. T3-T5 remain pending.
- Commit identity: pending.

## Next step
Implement T2c test-first: exercise the 90-second body-idle contract through a live local slow client and return a stable timeout response.
