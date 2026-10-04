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
- Accepted initial schema direction: `media` (client/idempotency ID, server storage name, media type, positive byte size, UTC confirmation time, visible/hidden state) and `settings` (key/value plus update time). The application layer must enforce an allowlist when settings operations are implemented; the base migration does not guess product keys. Add partial indexes for visible and hidden chronological gallery cursors. No upload-progress state, credentials, guest accounts, sessions, or speculative background-job/thumbnail tables.
- Keep docs under `docs/` in Spanish. Use test-first development for deterministic behavior changes.
- The user explicitly authorized implementing T1–T3 and creating one Conventional Commit per task on the feature branch.
- Review workload forecast: T1 alone adds 614 lines; the feature will exceed the repository's approximate 400-line review target. The user selected `feature-branch-chain` before the next work-unit commit. Planned delivery shape: draft tracker on `feat/backend-foundations` based on `develop`, with T2 and T3 as dependent child slices. T1 is already committed as a cohesive unit and exceeds the target; do not rewrite history or shrink tests/docs. PR creation remains unauthorized.
- Engram mirror topic: `odd/backend-foundations/tasks`.

## Tasks
- [x] **T1 — Add configuration and data-directory validation**
  - Route: delegated writer (multi-file behavior change); independent delegated verification; test-first.
  - Scope: parse and validate runtime configuration without leaking secrets; validate the existing data root, writable access, managed child directories, and filesystem constraints.
  - Files: `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/internal/storage/storage.go`, `backend/internal/storage/storage_test.go`.
  - Checks: RED — `cd backend && go test ./internal/config ./internal/storage` failed before implementation with undefined symbols; GREEN — same focused tests passed; independent `cd backend && go test ./... && go vet ./...` passed; scoped `git diff --check` reported no whitespace errors.
  - Evidence: tests cover configuration defaults/overrides and invalid/missing values; required-secret errors do not include secret values; storage tests cover missing/non-directory/unwritable roots, managed children and permissions, symlinks, and filesystem-ID matching/mismatch. CLI startup integration is intentionally deferred to T3. The requested existing data root is never created implicitly.
  - Commit: `9e84734` (`feat(backend): validate configuration and data directories`).

- [ ] **T2 — Add SQLite opening, migrations, and initial schema**
  - Route: delegated writer (multi-file behavior change); independent delegated verification; test-first.
  - Scope: add the approved pure-Go SQLite driver, configure SQLite pragmas, embed/apply versioned SQL migrations transactionally, and create the minimal `media` and `settings` schema. Application-layer settings allowlisting is documented as a requirement for future settings operations; no product keys are guessed here.
  - Files: `backend/go.mod`, `backend/go.sum`, `backend/internal/db/db.go`, `backend/internal/db/db_test.go`, `backend/internal/db/migrations.go`, `backend/internal/db/migrations/0001_initial.sql`, `docs/data-model.md`.
  - Checks: RED — `cd backend && go test ./internal/db` failed before implementation with undefined symbols; GREEN — same command passed; independent `cd backend && go test ./... && go vet ./...` passed; scoped checks found no whitespace errors.
  - Evidence: tests cover tables/columns/constraints/indexes, four simultaneous connections' pragmas, repeat-open behavior, transaction rollback, and newer-version rejection. The settings table stores no seeded values; SQL intentionally does not enumerate product keys, while data-model docs now require the application layer to enforce an allowlist and prohibit secrets.
  - Commit: pending.

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
- The user approved using the proposed initial schema as the baseline. Initial DDL design and application-layer settings-allowlist boundary are recorded above.
- Branch created from `develop`: `feat/backend-foundations`.
- Current worktree has a pre-existing untracked `.codegraph/` directory; preserve it.
- User authorization received to implement the feature and create one Conventional Commit per task.
- T1 implementation and independent verification completed; committed as `9e84734` (`feat(backend): validate configuration and data directories`).
- T2 implementation and independent verification passed; the initial verification found ambiguity about the `settings` allowlist. Resolved by documenting that the migration is generic and future application settings operations must enforce the allowlist; no product keys are invented.
- An out-of-scope `.codegraph/` ignore rule appeared in the root `.gitignore`; the exact added rule was removed and the pre-existing untracked `.codegraph/` directory is preserved.
- T2 implementation and independent verification passed; its `settings` allowlist boundary was clarified as an application-layer requirement for future settings operations. T2 work-unit commit is pending.
- User-selected delivery strategy: `feature-branch-chain`; no PR or child branch has been created.
- Commit identity: pending until each task is verified and committed.

## Review Workload and delivery
- Strategy: `feature-branch-chain` (user selected).
- Running authored-line count: T1 contributed 614 additions, already above the approximate 400-line target. T2 currently measures 499 changed lines before this note; one honest slicing pass found it is a cohesive DB foundation (driver, pooled pragmas, transactional migration machinery, initial schema, tests, and docs). Splitting it would leave an incomplete persistence contract, so the T2 child slice remains about 500 lines and roughly 100 over target. T3 remains a separate dependent child slice.
- Planned boundaries: tracker/base contains T1 on `feat/backend-foundations`; child slice 1 contains T2 and depends on T1; child slice 2 contains T3 and depends on T2. T1's existing cohesive commit is not rewritten; its overage is recorded.
- No PRs, pushes, or merges are authorized yet.

## Next step
Commit the verified T2 database foundation after checking its authored-line count and slice fit, then implement T3 CLI startup integration.
