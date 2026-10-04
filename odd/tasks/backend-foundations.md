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

- [x] **T2 — Add SQLite opening, migrations, and initial schema**
  - Route: delegated writer (multi-file behavior change); independent delegated verification; test-first.
  - Scope: add the approved pure-Go SQLite driver, configure SQLite pragmas, embed/apply versioned SQL migrations transactionally, and create the minimal `media` and `settings` schema. Application-layer settings allowlisting is documented as a requirement for future settings operations; no product keys are guessed here.
  - Files: `backend/go.mod`, `backend/go.sum`, `backend/internal/db/db.go`, `backend/internal/db/db_test.go`, `backend/internal/db/migrations.go`, `backend/internal/db/migrations/0001_initial.sql`, `docs/data-model.md`.
  - Checks: RED — `cd backend && go test ./internal/db` failed before implementation with undefined symbols; GREEN — same command passed; independent `cd backend && go test ./... && go vet ./...` passed; scoped checks found no whitespace errors.
  - Evidence: tests cover tables/columns/constraints/indexes, four simultaneous connections' pragmas, repeat-open behavior, transaction rollback, and newer-version rejection. The settings table stores no seeded values; SQL intentionally does not enumerate product keys, while data-model docs now require the application layer to enforce an allowlist and prohibit secrets.
  - Commit: `e56ca34` (`feat(backend): add SQLite migrations and initial schema`).

- [x] **T3 — Integrate shared initialization into CLI and document operations**
  - Route: delegated writer (multi-file behavior change); independent delegated verification.
  - Scope: make `boda check` and `boda serve` share configuration, storage validation, and database initialization; keep failures actionable and ensure database handles close correctly; document startup behavior and initial schema.
  - Allowed edit surfaces: `backend/cmd/boda/**`, `docs/runbook.md`.
  - Checks: RED — `cd backend && go test ./cmd/boda` failed before integration because `check` did not initialize or validate storage/SQLite; GREEN — same command passed after implementation; independent `cd backend && go test ./... && go vet ./... && go build ./cmd/boda` passed; `git diff --check` passed.
  - Evidence: tests cover successful `check` initialization/migration, secret-safe invalid configuration, missing data root, invalid DB path, and listener failure in `serve`. Both commands share initialization and defer DB close. Independent verifier found no code/docs defect; direct close-path instrumentation is not present, though deferred cleanup is visible in code. Spanish runbook documents the startup requirements. Native review of this exact 179-line candidate was medium risk, approved, and acknowledged; review authority was burned. ASSESS of the committed range remains unassessable due untracked-scope declaration, so no ASSESS-derived closure is claimed.
  - Commit: `8f0dac9` (`feat(backend): initialize storage for check and serve`).

- [ ] **T4 — Pass required runtime environment to the Compose smoke test**
  - Route: delegated writer (Compose and shell harness); baseline configuration assertion, then focused verification.
  - Scope: map the newly required BODA_DATA_DIR and secret settings into the app service; have `scripts/smoke.sh` supply deterministic fake test values, never real credentials. Use an existing writable in-container root for the ephemeral smoke data.
  - Allowed edit surfaces: `compose.yaml`, `scripts/smoke.sh`.
  - Checks: RED observed — `docker compose config --format json` with harmless test inputs showed only `BODA_ADDR` in `app.environment`; all four new required settings were absent. GREEN — pending `bash -n scripts/smoke.sh` and rendered config containing the required values. Do not run `scripts/smoke.sh` unless its Compose lifecycle is confirmed safe.
  - Evidence: delegated verifier observed the baseline failure; no files or Docker state were changed.
  - Commit: pending.

## Acceptance criteria
1. Startup rejects missing required configuration and an absent or unwritable data root without printing secret values.
2. Required child directories are created only beneath an existing data root, and `tmp/` and `originals/` are verified to be on the same filesystem.
3. SQLite opens with the accepted pragmas and applies the embedded initial migration exactly once, tracked through `PRAGMA user_version`.
4. The initial schema has only the agreed `media` and `settings` tables and the corresponding uniqueness/check/partial-index constraints; it has no `uploading` state or speculative product entities.
5. Both CLI commands use the same validated initialization path, with meaningful tests and Spanish operational/data-model documentation.
6. Go tests, vet, and build pass; failed, skipped, or unavailable checks are recorded accurately.
7. The Compose smoke harness passes valid test-only startup configuration to the app and does not embed production credentials.

## Progress and verification
- Exploration found an existing HTTP scaffold; configuration and migration directories are placeholders. See `backend/cmd/boda/main.go`, `backend/internal/config/`, and `backend/internal/db/migrations/`.
- The user approved using the proposed initial schema as the baseline. Initial DDL design and application-layer settings-allowlist boundary are recorded above.
- Branch created from `develop`: `feat/backend-foundations`.
- Current worktree has a pre-existing untracked `.codegraph/` directory; preserve it.
- User authorization received to implement the feature and create one Conventional Commit per task.
- T1 implementation and independent verification completed; committed as `9e84734` (`feat(backend): validate configuration and data directories`).
- T2 implementation and independent verification passed; the initial verification found ambiguity about the `settings` allowlist. Resolved by documenting that the migration is generic and future application settings operations must enforce the allowlist; no product keys are invented. Committed as `e56ca34` (`feat(backend): add SQLite migrations and initial schema`).
- An out-of-scope `.codegraph/` ignore rule appeared in the root `.gitignore`; the exact added rule was removed and the pre-existing untracked `.codegraph/` directory is preserved.
- RDD is on. ASSESS for committed T1/T2/T3 ranges was `unassessable` because native assessment required an explicit untracked-file declaration; independent verification passed for each task. For T1/T2, no native review closure is claimed. T3's exact worktree candidate was separately reviewed at medium risk, approved, and acknowledged; the acknowledgement burned authority. The committed-range ASSESS still did not derive closure. A committed-range START attempt for earlier candidates was rejected as a target mismatch before mutation.
- User-selected delivery strategy: `feature-branch-chain`; no PR or child branch has been created.
- Implementation task commits: T1 `9e84734`, T2 `e56ca34`, T3 `8f0dac9`.

## Review Workload and delivery
- Strategy: `feature-branch-chain` (user selected).
- Running authored-line count: T1 contributed 614 additions; T2 contributed 478 authored changed lines (499 total including 21 generated `go.sum` lines); T3 contributed 179 changed lines. Total: about 1,271 authored changed lines (1,292 including `go.sum`).
- T1 and T2 exceed the approximate 400-line target. One honest slicing pass found both are cohesive behavior units with tests/docs; splitting T1 or splitting the migration engine from the initial schema would create partial contracts. Their overages remain recorded rather than shrinking tests/docs or rewriting commits. T3 is under the target.
- All three commits are currently on `feat/backend-foundations`; no tracker/child branches or PRs have been created. If delivery is later authorized, verify the repository's default branch and shape a tracker plus dependent task slices without rewriting these commits.
- No PRs, pushes, or merges are authorized.

## Next step
Implement T4 in `compose.yaml` and `scripts/smoke.sh`, then verify the rendered environment without running the Compose lifecycle.
