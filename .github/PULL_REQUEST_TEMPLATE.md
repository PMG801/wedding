## Qué cambia y por qué

<!-- Resume el cambio y enlaza el issue: Closes #123 -->

## Cómo se ha comprobado

- [ ] `cd backend && go test ./... && go vet ./...`
- [ ] `cd frontend && npm test && npm run build`
- [ ] `scripts/smoke.sh` (si toca la API, el proxy o Docker)

## Lista de comprobación

- [ ] La rama sale de `develop` y el PR apunta a `develop`.
- [ ] Commits con Conventional Commits; unas 400 líneas o menos.
- [ ] Documentación de `docs/` actualizada si cambia el comportamiento.
- [ ] No incluye secretos, `.env`, datos ni fotos reales.
