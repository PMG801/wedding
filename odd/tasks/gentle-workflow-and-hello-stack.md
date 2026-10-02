# Gentle workflow setup and minimal hello stack

## Objective
Make the repo comfortable to work on with the Gentle/ODD harness (project-level persistent docs), then add a minimal runnable front + back example to exercise CI/CD and the team workflow.

## Decisions (user-confirmed)
- Branch: continue on `feat/minimum-cicd-template`; one work-unit commit per task.
- Docs: `AGENTS.md`, `odd/README.md` + task template, ADR convention (`odd/decisions` pointer + `docs/adr`).
- Example: Go `GET /api/health` + Svelte page that fetches it + local `docker compose` (app + web).
- CI/CD: keep existing CI, add an end-to-end smoke test (compose up, hit `/api/health` through Caddy). No image publishing.
- Non-goals: product features, registry publishing, Vitest major upgrade (deferred).

## Tasks
- [x] **T1 — Untrack stray binary, add project docs**: remove `backend/boda` from git, ignore it; add `AGENTS.md`, `odd/README.md`, `odd/templates/task.md`, ADR convention.
  - Evidence: git diff --check ok; backend/boda untracked. Commit: 492799b
- [x] **T2 — Backend HTTP server with `/api/health`** (test-first, `net/http`, stdlib only).
  - Evidence: RED (NewHandler undefined) -> GREEN; go test, go vet, build ok. Commit: 0baf458
- [x] **T3 — Frontend fetches and shows health** (Vitest test for the API client).
  - Evidence: RED (./health missing) -> GREEN; npm test 7/7, build, svelte-check ok. Commit: bf91d94
- [ ] **T4 — Docker Compose for local run** (`app` + `web`, healthchecks).
- [ ] **T5 — CI e2e smoke test + runbook/workflow docs update**.

## Evidence
(pending; record check results and commit ids per task)
