<p align="center">
  <img src="frontend/public/logo.svg" alt="Logotipo del proyecto" width="120">
</p>

<h1 align="center">Boda · Fotos y vídeos</h1>

<p align="center">
  Aplicación web para que los invitados de una boda suban fotos y vídeos desde el móvil,<br>
  escaneando un código QR, sin instalar nada ni crear cuentas.
</p>

<p align="center">
  <a href="https://github.com/PMG801/wedding/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/PMG801/wedding/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="Licencia MIT" src="https://img.shields.io/badge/licencia-MIT-7d9471.svg"></a>
</p>

## Qué hace

- **Acceso por QR.** El QR abre `/e/<token>` y crea una sesión de invitado; no hay registro.
- **Subida desde el móvil.** Fotos y vídeos (MP4, M4V, MOV) enviados en streaming, con reintento idempotente y barra de progreso. Las miniaturas se generan en el navegador.
- **Galería.** Cuadrícula con carga progresiva, visor a pantalla completa, «me gusta» anónimos y ranking de las más gustadas.
- **Publicaciones fijadas.** Mensajes del administrador, con imagen opcional de la galería.
- **Panel de administración** (`/admin`). Interruptor de subidas, estado del disco, moderación (ocultar y restaurar medios) y publicaciones fijadas.
- **Pensada para un servidor pequeño.** Un binario en Go, SQLite y Caddy; unos 100 MB de memoria en reposo. La entrega de los originales se hace con `rsync`, fuera de la aplicación.

## Arquitectura

| Pieza | Tecnología |
|---|---|
| Backend | Go (solo biblioteca estándar, `modernc.org/sqlite` y `golang.org/x/crypto`) |
| Persistencia | SQLite en modo WAL y medios en disco local |
| Frontend | Svelte 5, TypeScript y Vite (SPA estática) |
| Proxy | Caddy 2: sirve la SPA y los medios, y reenvía `/api/*` y `/e/*` |
| Empaquetado | Docker multi-stage (`app` y `web`), Docker Compose |

Más detalle en [`docs/architecture.md`](docs/architecture.md), [`docs/api.md`](docs/api.md), [`docs/data-model.md`](docs/data-model.md) y los [registros de decisiones](docs/adr/README.md).

## Puesta en marcha local

Requisitos: Docker con Compose.

```sh
cp .env.example .env
# Edita .env: sustituye los valores REEMPLAZAR_...
docker compose up --build
```

La web queda en <http://localhost:8081>. La entrada de invitado es `http://localhost:8081/e/<BODA_EVENT_TOKEN>`.

El token del evento debe codificar 32 bytes en base64url y la contraseña de administrador se guarda como hash bcrypt o argon2id. Para generar secretos y el hash, consulta [`deploy/server/bootstrap.sh`](deploy/server/bootstrap.sh) y el [runbook](docs/runbook.md).

## Desarrollo

```sh
# Backend (Go 1.23)
cd backend && go test ./... && go vet ./...

# Frontend (Node 22)
cd frontend && npm ci && npm test && npm run build

# Prueba de humo de extremo a extremo (necesita Docker Compose y curl)
scripts/smoke.sh
```

## Despliegue

El servidor no compila nada: descarga las imágenes `boda-app` y `boda-web` publicadas por CI en GHCR. El procedimiento completo (aprovisionamiento, HTTPS, copias de seguridad, entrega con `rsync` y operación durante el evento) está en [`docs/runbook.md`](docs/runbook.md).

## Personalizar la marca

El logotipo incluido es genérico. Para usar el de tu boda, sustituye los archivos de [`frontend/public/`](frontend/public/) conservando los nombres: `favicon.svg`, `logo.svg`, `apple-touch-icon.png` (180×180), `icon-192.png` y `icon-512.png`. Los textos («Nuestra boda», título de la pestaña) están en [`frontend/src/lib/i18n/es.ts`](frontend/src/lib/i18n/es.ts), y el título y el nombre de la app instalable, en [`frontend/index.html`](frontend/index.html) y `frontend/public/site.webmanifest`. La paleta está en [`frontend/src/app.css`](frontend/src/app.css).

La «W» del logotipo se ha dibujado con la fuente Fraunces (licencia SIL OFL 1.1), convertida a trazados.

## Contribuir

Las contribuciones son bienvenidas; lee antes [`CONTRIBUTING.md`](CONTRIBUTING.md). Para vulnerabilidades, sigue [`SECURITY.md`](SECURITY.md) y no abras un issue público.

## Licencia

[MIT](LICENSE).
