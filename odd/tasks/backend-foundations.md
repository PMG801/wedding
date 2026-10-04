# Backend foundations

## Objective
Prepare the first backend milestone for the wedding upload app: validated configuration and data storage, a reliable SQLite foundation, and the smallest approved initial schema.

## Problem and rationale
The repository currently has a runnable HTTP scaffold, but `boda check` only checks the working directory and `boda serve` starts without validating persistent storage or opening a database. Establish the persistence boundary before adding product behavior.

## Accepted scope and constraints
- Work branch: `feat/backend-foundations`, created from `develop`.
- Follow `docs/architecture.md`, `docs/runbook.md`, `docs/data-model.md`, and accepted ADRs 0002, 0003, and 0008; use only the `modernc.org/sqlite` database dependency beyond the existing allowed dependencies.
- Configuration is environment-based. Require the configured data root to exist and be writable; create the application's expected child directories beneath it with restrictive permissions, but never silently create a missing mount root. Verify `tmp/` and `originals/` share a filesystem.
- SQLite uses `data/boda.db`, WAL, `busy_timeout=5000`, `synchronous=NORMAL`, foreign keys, embedded numbered SQL migrations, and `PRAGMA user_version`.
- Accepted initial schema direction: `media` (client/idempotency ID, server storage name, media type, positive byte size, UTC confirmation time, visible/hidden state) and `settings` (allowlisted key/value plus update time); partial indexes for visible and hidden chronological gallery cursors. No upload-progress state, credentials, guest accounts, sessions, or speculative background-job/thumbnail tables.
- Keep docs under `docs/` in Spanish. Use test-first development for deterministic behavior changes.
- The user explicitly authorized implementing T1–T3 and creating one Conventional Commit per task on the feature branch.
- Engram mirror topic: `odd/backend-foundations/tasks`.

## Tasks
- [ ] **T1 — Add configuration and data-directory validation**
  - Route: delegated writer (multi-file behavior change); independent delegated verification; test-first.
  - Scope: parse and validate runtime configuration without leaking secrets; validate the existing data root, writable access, managed child directories, and filesystem constraints.
  - Files: `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/internal/storage/storage.go`, `backend/internal/storage/storage_test.go`.
  - Checks: RED — `cd backend && go test ./internal/config ./internal/storage` failed before implementation with undefined symbols; GREEN — same focused tests passed; independent `cd backend && go test ./... && go vet ./...` passed; scoped `git diff --check` reported no whitespace errors.
  - Evidence: tests cover configuration defaults/overrides and invalid/missing values; required-secret errors do not include secret values; storage tests cover missing/non-directory/unwritable roots, managed children and permissions, symlinks, and filesystem-ID matching/mismatch. CLI startup integration is intentionally deferred to T3. The requested existing data root is never created implicitly.
  - Commit: pending.

- [ ] **T2 — Add SQLite opening, migrations, and initial schema**
  - Route: delegated writer (multi-file behavior change); test-first.
  - Scope: add the pure-Go SQLite driver, configure SQLite pragmas, embed/apply versioned SQL migrations, and create the accepted minimal `media` and `settings` schema.
  - Allowed edit surfaces: `backend/go.mod`, `backend/go.sum`, `backend/internal/db/**`, `backend/internal/db/migrations/**`, `docs/data-model.md`.
  - Checks: migration/schema tests against a temporary database, pragma checks, repeated-open/idempotent migration test, migration failure handling, `cd backend && go test ./... && go vet ./...`.
  - Evidence: pending user authorization and implementation.
  - Commit: pending explicit user authorization.

- [ ] **T3 — Integrate shared initialization into CLI and document operations**
  - Route: delegated writer (multi-file behavior change); independent delegated verification.
  - Scope: make `boda check` and `boda serve` share configuration, storage validation, and database initialization; keep failures actionable and ensure database handles close correctly; document startup behavior and initial schema.
  - Allowed edit surfaces: `backend/cmd/boda/**`, `docs/runbook.md`, `docs/data-model.md`.
  - Checks: CLI/bootstrap tests; `cd backend && go test ./... && go vet ./... && go build ./cmd/boda`; verify `check` fails on invalid setup and succeeds on a prepared temporary data root.
  - Evidence: pending user authorization and implementation.
  - Commit: pending explicit user authorization.

## Acceptance criteria
1. Startup rejects missing required configuration and an absent or unwritable data root without printing secret values.
2. Required child directories are created only beneath an existing data root, and `tmp/` and `originals/` are verified to be on the same filesystem.
3. SQLite opens with the accepted pragmas and applies the embedded initial migration exactly once, tracked through `PRAGMA user_version`.
4. The initial schema has only the agreed `media` and `settings` tables and the corresponding uniqueness/check/partial-index constraints; it has no `uploading` state or speculative product entities.
5. Both CLI commands use the same validated initialization path, with meaningful tests and Spanish operational/data-model documentation.
6. Go tests, vet, and build pass; failed, skipped, or unavailable checks are recorded accurately.

## Progress and verification
- Exploration found an existing HTTP scaffold; configuration and migration directories are placeholders. See `backend/cmd/boda/main.go`, `backend/internal/config/`, and `backend/internal/db/migrations/`.
- The user approved using the proposed initial schema as the baseline. Initial DDL design is recorded above; no source implementation has begun.
- Branch created from `develop`: `feat/backend-foundations`.
- Current worktree has a pre-existing untracked `.codegraph/` directory; preserve it.
- User authorization received to implement the feature and create one Conventional Commit per task.
- T1 implementation and independent verification are complete; the T1 work-unit commit is being prepared.
- A writer briefly added `.codegraph/` to the root `.gitignore` outside its allowed surface; that exact unintended line was removed, and `.gitignore` is clean. The existing untracked `.codegraph/` directory is preserved.
- Commit identity: pending until each task is verified and committed.

## Next step
Implement T1 (configuration and data-directory validation) test-first, then continue task by task.
