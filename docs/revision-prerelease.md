# Revisión previa a la release

Revisión general del código, la configuración y el despliegue sobre `main` (commit `0f4796a`). Es de solo lectura: no se ha cambiado código. Sirve como lista de trabajo antes y después de la release.

## Estado de las comprobaciones

| Comprobación | Resultado |
|---|---|
| `go vet ./...`, `gofmt -l` | Sin avisos |
| `go test ./...` y `go test -race ./...` | Todo en verde |
| `govulncheck ./...` | 0 vulnerabilidades alcanzables. Hay 3 en módulos requeridos que el código no llama; una es GO-2026-6355 (`x/crypto/ssh`, corregida en `v0.56.0`) |
| `npm test` (287 tests) y `npm run build` | Todo en verde |
| `svelte-check` | 0 errores, 0 avisos (pero no forma parte de `npm` ni del CI) |
| `npm audit --omit=dev` | 0 vulnerabilidades en producción |
| `npm audit` (completo) | 8 avisos, todos de desarrollo (`vite` dev server, `tinypool` vía `vitest`); no llegan al bundle |

## Antes de la release

### Seguridad y robustez

1. **Cabeceras de seguridad ausentes en el Caddyfile (media).** Solo `/m/*` lleva `X-Content-Type-Options`. La SPA y la API no envían `Content-Security-Policy`, `Strict-Transport-Security`, `Referrer-Policy`, `X-Content-Type-Options` ni `frame-ancestors`/`X-Frame-Options`. Caddy no añade HSTS por sí solo. Propuesta: un bloque `header` común en `deploy/caddy/Caddyfile` (CSP con `default-src 'self'`, `img-src 'self' data: blob:`, `media-src 'self' blob:`, `frame-ancestors 'none'`).
2. **Reservas de subidas reanudables sin tope (media).** `NewUpload` reserva el tamaño declarado hasta `UploadResumeTTL` (24 h). Un invitado con la cookie puede crear muchas subidas de hasta 1,5 GiB sin enviar bytes y agotar `free - MinFree`, de modo que el resto recibe `insufficient_storage`. Además, `pendingResumableBytes` lee todos los `.info` bajo el mutex del ledger en cada admisión y en cada `/api/admin/status`. Propuesta: tope de subidas pendientes (número y bytes), y reducir el TTL de las que no han recibido ningún byte.
3. **Throttle de login global (baja, ya documentado).** Un tercero puede mantener el login del administrador en espera de hasta 30 s con intentos fallidos continuos. Es una decisión consciente (memoria constante), pero conviene saber que el administrador puede quedar bloqueado y que las peticiones esperan sin tope. Propuesta: limitar las esperas en cola (devolver 429 si hay más de N).
4. **Sesión de administrador sin revocación (baja, ya documentado).** El logout solo borra la cookie; la sesión dura 12 h. Cambiar `BODA_SESSION_KEY` invalida todas las sesiones (también las de invitados).
5. **«Me gusta» manipulables (baja, por diseño del ADR-0012).** Quien borra la cookie y vuelve a abrir `/e/{token}` obtiene un `guest_id` nuevo y puede votar otra vez. Aceptable con ~110 invitados conocidos.
6. **Imagen base y dependencias de CI sin fijar (baja).** `node:22-alpine`, `golang:1.25-alpine`, `caddy:2-alpine` y las acciones de GitHub se referencian por etiqueta, no por digest/SHA. El despliegue usa la etiqueta mutable `rc`; para la boda conviene desplegar la etiqueta semver de la release.

### Dependencias

- `cd backend && go get golang.org/x/crypto@v0.56.0 && go mod tidy`.
- `cd frontend && npm audit fix` (arregla `vite` sin cambios incompatibles). `tinypool` exige `vitest@5` (cambio mayor); puede esperar porque es solo de desarrollo.

### Detalles menores de código

- `cmd/boda/main.go`: `err == http.ErrServerClosed` debería ser `errors.Is`.
- `cmd/boda/main.go`: tras un error de `Reconcile` se registra además el mensaje `reconciled upload storage` con el informe vacío.
- `Dockerfile`: la etapa del backend copia solo `go.mod` antes de `go mod download`; copiar también `go.sum` mejora la caché y verifica la integridad.
- `deploy/server/bootstrap.sh` obtiene el hash con la imagen `boda-web:rc` (etiqueta mutable).

## Frontend: estilo y buenas prácticas

Puntos que están bien: tokens de color en `:root`, `min-height` de 44 px en la navegación y los botones, `prefers-reduced-motion` en todas las animaciones, `100dvh`, imágenes con `loading="lazy"` y `decoding="async"`, todos los `<button>` con `type`, sin `{@html}`, sin `outline: none`, sin `console.log`, textos en `lib/i18n`.

A corregir o mejorar:

1. **Contraste (la más importante).** El texto blanco sobre `--sage` (`#7d9471`) del `.button.primary` da **3,31:1**; WCAG AA pide 4,5:1 para texto normal. Oscurecer el fondo (por ejemplo usar `--sage-dark`, 5,67:1 con `--paper`) o crear un token `--sage-strong`.
2. **Tokens incompletos.** Siguen en literal `#fff`, `#1d1a17`, `rgb(47 42 38 / …)` (sombras, 5 sitios), `rgb(29 26 23 / …)`, `rgb(20 17 15 / …)` y `rgb(255 253 248 / …)`. Añadir `--on-sage`, `--shadow-*`, `--scrim-*` y `--viewer-bg`.
3. **Sin escala tipográfica.** Hay ~25 valores de `font-size` distintos (0,72, 0,75, 0,8, 0,82, 0,85, 0,9, 0,95 rem…). Definir 5-6 pasos (`--text-xs … --text-xl`). `0.72rem` (≈11,5 px) en las etiquetas de la navegación queda por debajo de 12 px.
4. **`z-index` sin escala.** Valores sueltos 1, 15, 19 y 20 repartidos entre `AlbumSection`, `BackToTop`, `PhotoDialog` y `GuestHome`. Centralizarlos en variables (`--z-sticky`, `--z-nav`, `--z-top`).
5. **Foco inconsistente.** Solo `.button`, `BottomNav`, `GalleryGrid`, `PinnedSection` y `CarouselStrip` definen `:focus-visible`. En `AlbumSection`, `HiddenMediaList`, `MediaLightbox`, `PhotoDialog`, `PinnedEditor`, `StoryViewer`, `UploadPanel` y las vistas se usa el anillo por defecto del navegador. Funciona, pero conviene una regla global `:focus-visible` única.
6. **Objetivos táctiles.** `.button.small` mide 2,25 rem (36 px), por debajo de los 44 px de la guía de Apple/Material, y se usa en acciones de administración (mover, editar, borrar).
7. **`prefers-reduced-motion` repetido.** Se declara componente a componente. Una regla global (`*, *::before, *::after { animation-duration: .01ms !important; transition-duration: .01ms !important }`) cubre también lo que se añada después.
8. **Sin herramientas de estilo.** No hay ESLint, Prettier ni stylelint, y `svelte-check` no está en `package.json`. Añadir los scripts `check` y `lint` y ejecutarlos en el CI.

Estado: los puntos 2 a 7 y el script `check` del punto 8 están resueltos (ver «Convenciones de estilo» en `docs/architecture.md`); `lint` sigue pendiente.

## Futuro: calidad y seguridad en el CI/CD

Apuntado para una implementación posterior (no forma parte de la release).

**Hoy el CI ejecuta:** `go test`, `go vet`, compilación, `npm test`, `npm run build`, build y publicación de imágenes, y `scripts/smoke.sh`. Gaps detectados en `.github/workflows/ci.yml`:

- No se dispara con `pull_request` (solo con `push` a `develop`/`main` y con `release`), así que los checks corren **después** de fusionar. La clave `concurrency` ya contempla PR; decidir si se reactiva.
- `permissions: packages: write` está a nivel de workflow; debería limitarse al job de publicación.
- Las imágenes se publican **antes** del smoke test y sin escaneo.

**Herramientas candidatas** (de más a menos prioridad):

1. **SonarQube Community / SonarCloud** (análisis estático, duplicación, cobertura, *security hotspots*, *quality gate* en PR). Soporta Go y TypeScript/Svelte; necesita informe de cobertura (`go test -coverprofile`, `vitest --coverage`). SonarCloud es gratuito para repositorios públicos; SonarQube Community exige un servidor propio, que aquí pesa más que el beneficio.
2. **`govulncheck`** y **`npm audit --omit=dev`** como pasos bloqueantes (ya ejecutados a mano en esta revisión).
3. **CodeQL** (GitHub Advanced Security, gratis en repositorios públicos) para Go y JavaScript/TypeScript.
4. **Trivy** o **Grype** sobre las imágenes `boda-app` y `boda-web` antes de publicarlas; incluir SBOM y *provenance* en `docker/build-push-action`.
5. **`golangci-lint`** (con `staticcheck`, `gosec`, `errorlint`) y **ESLint + `eslint-plugin-svelte` + Prettier + stylelint** para el frontend.
6. **Dependabot** o **Renovate** (Go, npm, GitHub Actions, imágenes base) y fijar las acciones por SHA.
7. Pasos baratos que ya se pueden añadir: `go test -race ./...` y `svelte-check` en el CI.

**Orden sugerido:** (7) → (2) → (5) → (4) → (6) → (3) → (1). Sonar aporta más valor cuando ya hay cobertura y linters estables, y evita ruido inicial.
