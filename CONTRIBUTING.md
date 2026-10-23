# Contribuir

Gracias por querer mejorar el proyecto. Este documento resume cómo trabajar en él.

## Antes de empezar

- Abre un issue antes de empezar un cambio grande, para acordar el enfoque. El alcance del producto es deliberadamente pequeño: subir y ver fotos y vídeos en una boda. No se aceptarán funciones que lo amplíen mucho (cuentas de usuario, redes sociales, edición de medios…).
- Lee [`docs/architecture.md`](docs/architecture.md) y los [ADR](docs/adr/README.md). Las decisiones aceptadas se respetan; si crees que una debe cambiar, propón un ADR nuevo.

## Entorno

- Backend: Go 1.23. Solo biblioteca estándar, salvo el driver de SQLite y `golang.org/x/crypto`; no añadas más dependencias sin discutirlo.
- Frontend: Node 22, Svelte 5, TypeScript y Vite.
- Docker con Compose para la prueba de humo.

## Comprobaciones obligatorias

Tu PR debe pasar estas comprobaciones, las mismas que ejecuta CI:

```sh
cd backend && go test ./... && go vet ./...
cd frontend && npm ci && npm test && npm run build
scripts/smoke.sh
```

Si cambias comportamiento, escribe antes la prueba que falla y después el arreglo mínimo. Actualiza la documentación de `docs/` y el texto de ayuda cuando cambies un comando, una opción o un mensaje.

## Ramas y commits

- `develop` es la rama de integración. Crea tu rama desde ella con el prefijo `feat/`, `fix/` o `chore/` y abre el PR hacia `develop`.
- Usa [Conventional Commits](https://www.conventionalcommits.org/es/): `feat(frontend): …`, `fix(backend): …`.
- Un commit por unidad de trabajo, con sus pruebas y su documentación.
- Mantén cada PR en unas 400 líneas cambiadas o menos. Divide los cambios mayores en PR encadenados.

## Idioma

- La documentación de `docs/` y la de cara al usuario está en español.
- El código, los comentarios, los identificadores y los mensajes de commit están en inglés.

## Datos privados

No subas nunca fotos reales de invitados, archivos `.env`, bases de datos, copias de seguridad ni secretos. `.env.example` solo debe llevar valores de ejemplo.
