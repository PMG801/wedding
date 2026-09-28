# Minimum CI/CD Project Template

## Objective
Create a minimal, executable project scaffold and CI workflow aligned with the delivered architecture, so pull requests into `develop` and merges/pushes to `develop` run backend and frontend checks and build the Fase 1 Docker images.

## Problem and rationale
The repository initially contained documentation and empty directories only. It had no Go module/source, Node manifest/source, Dockerfile, or CI workflow, so a workflow added alone would fail immediately. The user approved a minimal runnable scaffold without product features.

## Accepted scope and constraints
- CI provider: GitHub Actions.
- Events: pull requests targeting `develop` and pushes to `develop` (including merge commits).
- Build backend and frontend, run their available automated tests, and build both Docker image targets.
- Docker target: Fase 1 minimal; support documented `linux/arm64` and `linux/amd64` build platforms.
- Build only: do not publish images, deploy, or require registry credentials/secrets.
- Follow `docs/architecture.md`, `docs/runbook.md`, and applicable ADRs; preserve Go + Svelte 5/TypeScript/Vite + Caddy architecture.
- Bootstrap only the minimal executable skeleton needed by CI. Do not implement wedding/media product functionality.
- TDD mode: ordinary functional checks, explicitly selected by the user. No project/session TDD configuration was found.
- Existing untracked `.gitignore` predates this work; preserve it.
- Work branch: `feat/minimum-cicd-template` (created from `develop`). No commit, push, PR, or merge is authorized by the user.

## Tasks

- [x] **T1 — Add backend starter and checks**
  - Route: delegated direct writer; multi-file implementation; independent delegated verification after Go became available.
  - Scope: minimal Go module/CLI health-check entry point and focused tests, consistent with the documented `boda check` command.
  - Files: `backend/go.mod`, `backend/cmd/boda/main.go`, `backend/cmd/boda/main_test.go`.
  - Checks: `cd backend && go test ./...` — passed; `cd backend && go vet ./...` — passed; `cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/boda-linux-arm64 ./cmd/boda` — passed.
  - Evidence: independent verifier observed all three exit 0. The initial unavailable-toolchain blocker was resolved after the user installed Go.

- [x] **T2 — Add frontend starter and checks**
  - Route: delegated direct writer; multi-file implementation.
  - Scope: minimal Svelte 5 + TypeScript + Vite app, lockfile, and a small automated test; no product feature UI.
  - Files: frontend package/lock manifests, Vite/Svelte/TypeScript/Vitest configuration, app entry/component/styles, and tested greeting utility.
  - Checks: `cd frontend && npm ci` — passed (117 packages); `cd frontend && npm test` — passed (3 tests); `cd frontend && npm run build` — passed.
  - Evidence: writer executed all three commands successfully. No commit created because the user did not explicitly request one.

- [x] **T3 — Add Fase 1 Docker image targets**
  - Route: delegated direct writer; multi-file implementation; delegated read-only verification of both multi-platform image builds after Docker socket access became available.
  - Scope: multi-stage Docker build for backend (`app`) and static frontend behind Caddy (`web`), without Fase 2 media tooling.
  - Files: `Dockerfile`, `.dockerignore`, `deploy/caddy/Caddyfile`. The writer removed Fase 2-specific media routes from Caddy while retaining static SPA serving and the documented `/api/*` proxy route.
  - Checks: `docker buildx build --platform linux/amd64,linux/arm64 --target app --tag boda-app:ci --output type=oci,dest=/tmp/boda-app.oci.tar .` — passed (exit 0; 2.3 MB OCI archive); corresponding `--target web --tag boda-web:ci --output type=oci,dest=/tmp/boda-web.oci.tar .` — passed (exit 0; 45 MB OCI archive). Both targets built for `linux/amd64` and `linux/arm64`; neither command pushed an image.
  - Integration boundary: Caddy routes `/api/*` to `app:8080`, while the current backend only implements the `boda check` CLI and does not listen on HTTP. This does not block image-build CI, but end-to-end Compose runtime validation would require additional scope.
  - Evidence: independent verifier ran both exact CI-equivalent OCI export commands successfully. Repository worktree status was unchanged.

- [x] **T4 — Add GitHub Actions CI and workflow documentation** (Vitest advisory deferred by user)
  - Route: delegated direct writer; multi-file implementation; independent read-only verification.
  - Scope: checks on PRs into and pushes to `develop`; test/build jobs; build-only Docker validation; concise runbook guidance; ignore generated frontend dependencies/build outputs without modifying the pre-existing untracked root `.gitignore`.
  - Files: `.github/workflows/ci.yml`, `docs/runbook.md`, `frontend/.gitignore`.
  - Checks: workflow triggers/jobs confirmed; backend tests/vet/ARM64 compile passed; frontend install/tests/build passed; both Docker targets built successfully for linux/amd64 and linux/arm64 as recorded under T3.
  - Evidence: independent verifier confirmed functional checks and workflow settings. `npm audit` reports two moderate entries for GHSA-82fw-gwwq-j7x9 affecting direct `vitest` and transitive `@vitest/mocker`; the reported fix is Vitest 5.0.2, a major-version upgrade. No update was applied; the user chose to defer the major upgrade and accept the moderate advisory as a follow-up. The esbuild install-script warning from `npm ci` did not appear in audit output.

- [x] **T5 — Re-run backend checks when Go toolchain is available**
  - Route: delegated read-only verifier.
  - Scope: resolve the T1 verification blocker after the user installed Go.
  - Checks: all T1 test, vet, and Linux ARM64 build commands passed with exit 0.
  - Evidence: verifier observed `go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ...` successful.

- [x] **T6 — Assess reported frontend dependency warnings**
  - Route: delegated read-only verifier; no dependency changes made.
  - Scope: identify affected dependencies and available fixes without changing package files.
  - Check: `cd frontend && npm audit --json` — exit 1; two moderate entries for GHSA-82fw-gwwq-j7x9, affecting Vitest and `@vitest/mocker`.
  - Evidence: npm offers Vitest 5.0.2 as the fix, which is a major-version upgrade. The esbuild install-script warning did not appear in the audit result.

- [x] **T7 — Decide whether to upgrade Vitest**
  - Route: user decision.
  - Scope: decide whether to apply the offered major upgrade or defer it.
  - Evidence: user chose to defer Vitest 5.0.2. Current moderate advisory remains documented; no dependency changes made.

## Acceptance criteria
1. The documented technology stack has a minimal compilable backend and buildable frontend scaffold, with tests executable from CI.
2. `.github/workflows/ci.yml` runs for pull requests targeting `develop` and pushes to `develop`.
3. CI runs backend tests, static analysis, and compilation; frontend tests and production build; and builds `app` and `web` Docker targets for `linux/arm64` and `linux/amd64`.
4. The Docker workflow neither logs into a registry nor pushes or deploys images.
5. Fase 1 remains minimal and does not add `libvips`/`ffmpeg` or product features.
6. All required checks are observed and failures/unavailable checks are recorded accurately.

## Progress and verification
- Exploration: delegated documentation and repository mapping completed. The repository began as a skeleton with no source or build manifests.
- Decisions confirmed: GitHub Actions; build only; Fase 1; include a runnable minimal scaffold; ordinary functional checks.
- Verification evidence: frontend `npm ci`, `npm test` (3/3), and `npm run build` passed; backend test, vet, and ARM64 cross-build all passed after Go was installed; `app` and `web` Docker targets both built for linux/amd64 and linux/arm64 and exported successfully as OCI tarballs (2.3 MB and 45 MB respectively). The frontend Docker build reports the previously identified moderate Vitest-related npm advisories; they did not block the build, and the user chose to defer the offered Vitest major-version upgrade.
- Commit identity: pending. The user did not explicitly request a commit, so the explicit no-commit boundary is preserved.

## Next step
All required local checks and both multi-platform Docker image builds have passed. The previously deferred moderate Vitest advisory remains a follow-up by user decision. No commit, push, PR, or merge was requested; leave delivery to the user.