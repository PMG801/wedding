# Project guidance

## Project and source of truth

This is a wedding photo/video upload app. Before making architectural or operational changes, read `docs/architecture.md`, `docs/runbook.md`, and the relevant records under `docs/adr/`.

## Stack and boundaries

- Backend: Go in `backend/`, using the standard library; SQLite is planned for persistence.
- Frontend: Svelte 5, TypeScript, and Vite in `frontend/`.
- Caddy serves the frontend and proxies API requests. Docker uses multi-stage targets `app` and `web`.
- Keep Go dependencies to the standard library except the SQLite driver and `golang.org/x/crypto`.
- Avoid product-scope creep. Generated artifacts are in English. Documentation under `docs/` is Spanish; keep it Spanish.

## Commands

- Backend: `cd backend && go test ./... && go vet ./...`
- Frontend: `cd frontend && npm ci && npm test && npm run build`
- Docker backend image: `docker build --target app -t boda-app .`
- Docker web image: `docker build --target web -t boda-web .`

## Git and delivery

- `develop` is the integration branch. Use feature branches `feat/*`, `chore/*`, or `fix/*`; open pull requests into `develop`.
- Use Conventional Commits. Make one work-unit commit per task, keeping its tests and documentation with the change.
- Never push or merge without the user's explicit instruction.
- Keep a review around 400 changed lines or fewer. Split larger work according to the chained-PR skill.

## ODD and implementation

- Track substantial work in `odd/tasks/<feature>.md`, with an Engram mirror under topic `odd/<feature>/tasks`; see `odd/README.md` for the workflow.
- For behavior changes with applicable deterministic tests, use test-first development: observe the failing test, implement the smallest fix, then verify relevant alternatives.
- Follow existing conventions and accepted ADRs. Record task evidence and verification in the task file.
