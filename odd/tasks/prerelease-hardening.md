# Pre-release hardening

Fixes chosen from `docs/revision-prerelease.md` before the release. Disk space comes first: one disk is shared by data and OS.
Each task is its own branch from `develop` (`fix/*` or `chore/*`), one work-unit commit, no push, no merge.

## Specs

- **S1** Pending resumable uploads have a global cap. `BODA_MAX_PENDING_UPLOADS` (positive integer, default `300`). Creating an upload when the cap is reached answers HTTP 503, header `Retry-After: 30`, body `{"error":"too_many_pending_uploads"}`, and creates no file. The order of checks in `resumableCore.NewUpload` stays: invalid ID (400), duplicate (409 `upload_in_progress`), then the cap, then the free-space admission (507 `insufficient_storage`).
- **S2** A pending upload that has received zero bytes expires sooner. `BODA_UPLOAD_EMPTY_TTL` (positive duration, default `15m`). Reconciliation removes it once its files are older than that. An upload with at least one byte keeps using `BODA_UPLOAD_RESUME_TTL`. An effective empty TTL is never longer than the resume TTL.
- **S3** Staged derivatives of unconfirmed originals have a cap derived from S1: at most `2 × BODA_MAX_PENDING_UPLOADS` `.staged` files in `tmp/`. A `POST /api/media/{id}/derived` that would exceed it answers HTTP 503, header `Retry-After: 30`, body `{"error":"too_many_staged_derivatives"}` and writes nothing. A retry of a derivative that is already staged or published still answers as before (200).
- **S4** The free-space protection that exists today does not change: `BODA_DISK_MIN_FREE_BYTES`, reservations, `507 insufficient_storage`, and the admin status numbers keep their current behavior.
- **S5** Caddy sends security headers on the SPA and the API: `Content-Security-Policy` (`default-src 'self'`, images and media from `'self'`, `data:` and `blob:`, `frame-ancestors 'none'`, `base-uri 'self'`, `form-action 'self'`), `Strict-Transport-Security` with a short `max-age` and no `includeSubDomains` or `preload`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`. The app must keep working under that CSP.
- **S6** The primary button reaches WCAG AA contrast: white text on `--sage` is 3.31:1 and must be at least 4.5:1.
- **S7** Dependencies: `golang.org/x/crypto` to `v0.56.0`; `npm audit fix` without `--force`; `vitest@5` is not part of this.

## Tasks

- [x] **T1** (S1, S2, S4) Pending-upload cap and empty-upload TTL. Route: delegated writer in worktree `.worktrees/upload-cap`, then independent verify (high risk item 4, concurrency): S1, S2, S4 pass; hammer test (cap 5, 60 concurrent creations, 20 runs under `-race`) always accepted 5. Branch `fix/upload-pending-cap`. Commit: `f154dce` (shared with T2).
- [x] **T2** (S3, S4) Staged derivative cap. Same worktree, branch and commit as T1 (312 lines in total, so one slice). Verifier: S3 pass; the published-derivative retry over the cap is verified by reading only. Commit: `f154dce`.
- [x] **T3** (S5) Caddy security headers. Inline. Branch `fix/caddy-security-headers`. Checks: `caddy validate`, headers on `/`, `/api/health`, a 404 and an SPA path, Chromium with the real CSP shows no violations (a strict control CSP shows 7), `scripts/smoke.sh` exit 0 (its first run exited 1 and the cause was not captured). Commit: `ff4c8fe`.
- [x] **T4** (S6) Primary button contrast. Inline. Branch `fix/primary-button-contrast`. RED 3.31 < 4.5 on the old CSS, GREEN with `--sage-dark`; 289 tests, build, svelte-check 0 errors. Not looked at in a browser. Commit: `6cb82dd`.
- [x] **T5** (S7) Dependency updates. Inline. Branch `chore/dependency-updates`. `vite` 6.4.5 via `npm audit fix` (0 production vulnerabilities, 287 tests, build, svelte-check). **Deviation:** `golang.org/x/crypto` v0.56.0 requires Go 1.26 (`go get` rewrote `go 1.25.14` to `go 1.26.0`); the project builds with Go 1.25, so it was reverted. The advisory GO-2026-6355 is in `x/crypto/ssh`, which the code does not call. Needs a decision: move the project to Go 1.26 (go.mod, Dockerfile, CI). Commit: `9832739`.
- [x] **T6** Notes and branch rule. `docs/revision-prerelease.md` was already in `develop` (committed by the user as `f8cda0b`). This file and an explicit branch line in `AGENTS.md`. Branch `chore/prerelease-review-notes`. Commits: `46c5d13` (AGENTs), this file in the next commit.

## Log

- **L1** (user) "Ya tengo las funcionalidades que buscaba. revisa en general el proyecto en busca de fallos mejoras etc. Antes de sacar la release, vulnerabilidades malas practicas. Apunta para una futura implementacion en el cicd meter un sonar o una herramienta de ese estilo. Y para la parte de front un repaso del estilo que este bien puesto en todas las partes y que siga las buenas practicas"
- **L2** (user) "Antes de aceptar el plan dime un poco mas claro que se va a hacer"
- **L3** (user) "Ejecuta, lo que mas me importa es el tema del espacio, ya que esto va a correr en una maquina donde hay un solo disco por lo que la data y el SO van a compartir disco y si se llena la maquina se jode y cuesta mas de recuperar. Recuerda el tema de trabaajr en una rama que salga de develop (no me acuerdo si esta en las instrucciones del proyecto, pero si no esta ponlo)"
- **L4** (parent) Audit result: disk admission (`MinFreeBytes` plus reservations) already stops the disk from filling past the threshold. What remains is availability: zero-byte reservations (24 h) and unbounded staged derivatives can exhaust the admission budget and pause uploads for everyone. S1-S3 close that.
- **L5** (parent) Verifier defects left open, all low: D1 a derivative is counted twice between its rename and the deferred decrement (one spurious 503 at cap-1, never over the cap); D2 `os.ReadDir(tmp)` runs under the ledger mutex; D4 (pre-existing) a PATCH on an expired upload gets tusd's plain-text 404, not the app's JSON. D3 (`.env.example` keys commented out) was fixed before the commit.
