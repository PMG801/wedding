# Resumable tus uploads (and dropping the Cloudflare edge)
Status: IN PROGRESS. Branch `feat/tus-uploads` from `develop` (`721a9fc`). Sources of truth: `docs/architecture.md`, ADR-0005, ADR-0008, ADR-0009, ADR-0011.
Tests: `cd backend && gofmt -l . && go vet ./... && go test -race ./...`; `cd frontend && npm test && npm run build && npx svelte-check`; `scripts/smoke.sh`.

## Decisions (D1-D4 chosen by the user, L2)
- **D1 — tus server.** Use the `github.com/tus/tusd/v2` library (`pkg/handler`, `pkg/filestore`, `pkg/memorylocker`) and record the dependency exception in an ADR and in `AGENTS.md`. Non-test footprint: tusd + `golang.org/x/exp`; tusd v2.10.1 requires Go 1.25 (go.mod and `Dockerfile` `GO_VERSION` bump).
- **D2 — Scope.** Every original (photo and video) goes through tus. Client derivatives (`POST /api/media/{id}/derived`) keep their short request.
- **D3 — Middle cache.** Discarded; the reason is documented in `docs/architecture.md` (1.6).
- **D4 — Order.** Cloudflare removal first, then tus.

## Specs
- **S1** Remove Cloudflare and WAF from the project. L1: "Al final voy a quitar toda la parte de cloudflare y temas wafs he decidido que lo voy a gestionar yo directamente accediendo directamente a la maquina, obviamente poniendo las buenas practicas." Host hardening stays with the user, outside the repository.
- **S2** Heavy photo and video uploads use tus. L1: "quiero adaptar la subidas de imagenes y videos mas pesados con el protocolo tus para de esta forma poder segmentarlos y tener herramientas apra recuperar en caso de fallo."
- **S3** Middle cache. L1: "Otra cosa es la de meter algo de cache entre media para intentar ganar algo de rendimiento, pero sinceramente no estoy seguro si vale la pena" → D3, discarded.

## Tasks
| ID | Spec | Route | Risk | Commit |
|---|---|---|---|---|
| T1 | S1 | inline: confirm `develop` has no Cloudflare; drop the `cloudflare-edge` plan; local branches `chore/cloudflare-tunnel`, `docs/cloudflare-plan` deleted only with user OK | none | n/a (nothing on `develop`) |
| T2 | S2, S3 | inline: ADR-0014 (tus contract, amends ADR-0005/0008/0009), architecture 1.2/1.6, `AGENTS.md` dependency rule, `docs/api.md` contract | item 3 (upload contract) | pending |
| T3 | S2 | worker, test-first: backend tus endpoint, publish on finish, reservation rebuilt from pending uploads, expiry cleanup, Go 1.25 | items 1, 5 | pending |
| T4 | S2 | worker, test-first: frontend originals through `tus-js-client` with resume | item 3 | pending |
| T5 | S2 | inline: remove `PUT /api/media/{id}`, runbook, smoke resume check | item 3 | pending |

## Log
- **L1** (user, 2026-11-01): "Al final voy a quitar toda la parte de cloudflare y temas wafs he decidido que lo voy a gestionar yo directamente accediendo directamente a la maquina, obviamente poniendo las buenas practicas. A parte de esto quiero adaptar la subidas de imagenes y videos mas pesados con el protocolo tus para de esta forma poder segmentarlos y tener herramientas apra recuperar en caso de fallo. Otra cosa es la de meter algo de cache entre media para intentar ganar algo de rendimiento, pero sinceramente no estoy seguro si vale la pena"
- **L2** (user answers, 2026-11-01): tus server "Librería tusd + ADR"; scope "Todos los originales"; cache "Descartar"; order "Quitar Cloudflare primero".
- **L3** (parent, T1 evidence): `git grep -niE "cloudflare|cloudflared|tunnel" develop` returns nothing; `06b2781` (tunnel) and `93e97d7` (edge plan) live only on local `chore/cloudflare-tunnel` and `docs/cloudflare-plan`, never pushed (`git ls-remote` shows no such heads). `develop` already serves directly through Caddy (`BODA_DOMAIN`, ports 80/443 opened by `provision-host.sh`). The production server must drop its `cloudflared` container and open 80/443 (user, on the host).
