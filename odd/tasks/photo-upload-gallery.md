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
- [ ] **T1 — Secure guest QR session** (in progress)
  - Route: one bounded `gentle-ai-worker` implementation; parent coordinates and records evidence.
  - Scope: implement `/e/{token}` validation, signed guest cookie, protected-handler seam, server dependency wiring, and safe local/runbook token provisioning; include focused tests and Spanish runbook documentation.
  - Files: `backend/internal/auth/**`, `backend/internal/httpapi/**`, `backend/internal/config/**`, `backend/cmd/boda/**`, `backend/**_test.go`, and `docs/runbook.md`.
  - Checks: focused auth/HTTP tests; `cd backend && go test ./... && go vet ./...`.
  - Evidence: TDD RED observed with `cd backend && go test ./internal/auth ./internal/httpapi`; targeted GREEN and triangulation passed with `go test ./internal/auth ./internal/httpapi` and `go test ./internal/auth ./internal/httpapi ./cmd/boda`. Independent verification passed `cd backend && go test ./... && go vet ./...` and `git diff --check`. Verified 273 changed lines (257 additions, 16 deletions), within the review budget. Secure-cookie local use requires HTTPS; no insecure exception was added.
  - Commit: pending.
- [ ] **T2 — Stream and persist photo originals**
  - Route: one bounded `gentle-ai-worker`; tests and API documentation stay with the behavior.
  - Scope: add the DB/media repository and photo upload endpoint with ID/type/signature/size checks, disk and concurrency limits, bounded streaming, atomic confirmation, and idempotent retries.
  - Files: `backend/internal/upload/**`, `backend/internal/media/**`, `backend/internal/db/**`, `backend/internal/httpapi/**`, `backend/cmd/boda/**`, `backend/**_test.go`, and `docs/api.md`.
  - Checks: focused validation/storage/HTTP tests; `cd backend && go test ./... && go vet ./...`.
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
- Review workload forecast: the full vertical slice spans backend, frontend, proxy, and documentation and is expected to exceed 400 changed lines; use the five bounded deliverables above and record actual slice size before each commit.
- Progress: T1 implementation and independent full backend verification are complete; awaiting its local work-unit commit. The remaining four tasks are pending.
- Commit identity: pending.

## Next step
Create the T1 work-unit commit on the current feature branch, then mark T1 complete and begin T2.
