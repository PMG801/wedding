# Runbook

## Ejecución local con Docker Compose

Desde la raíz del repositorio, copia `.env.example` a `.env`, reemplaza los valores de ejemplo y construye y arranca la aplicación y Caddy:

```sh
cp .env.example .env
# Edita .env y sustituye los valores REEMPLAZAR_...
docker compose up --build
```

La web queda disponible en <http://localhost:8081>. Comprueba la API y que se sirve la página:

```sh
curl -fsS http://localhost:8081/api/health
curl -fsS http://localhost:8081/ | head
```

## Despliegue en el servidor (`/srv/boda`)

Todo el despliegue vive en `/srv/boda` y su fuente de verdad es `deploy/server/` del repositorio. El servidor no compila nada: descarga de GHCR las imágenes `boda-app` y `boda-web` con la etiqueta `rc`, que CI publica en cada push a `main` (se puede fijar otra con `BODA_TAG`).

Preparación única (requiere Docker Engine y el plugin de Compose instalados):

```sh
sudo install -d -o "$USER" -g "$USER" /srv/boda
scp deploy/server/{compose.yaml,deploy.sh,bootstrap.sh} <servidor>:/srv/boda/
ssh <servidor> 'cd /srv/boda && ./bootstrap.sh'
```

`bootstrap.sh` crea `data/` (propiedad del UID `65532`, modo `0700`) y genera un `.env` con permisos `600` y secretos aleatorios, más `.admin-password`. Es idempotente: nunca sobrescribe un `.env` existente. Ninguno de los dos archivos está en Git.

Para desplegar o actualizar: `cd /srv/boda && ./deploy.sh` (hace `docker compose pull` y `up -d`).

Mientras no haya dominio ni HTTPS, la web solo escucha en `127.0.0.1:8081` del servidor (variables `BODA_BIND` y `BODA_HTTP_PORT`). Se accede con un túnel SSH, porque la cookie `Secure` solo se acepta en HTTPS o en `localhost`:

```sh
ssh -L 8081:127.0.0.1:8081 <servidor>
# QR de prueba: http://localhost:8081/e/<BODA_EVENT_TOKEN>
```

Con dominio: define `BODA_DOMAIN=<dominio>` en `.env`, `BODA_BIND=0.0.0.0` y publica además los puertos 80 y 443.

## Integración continua

GitHub Actions ejecuta la verificación en cada push a `develop` o `main` y al publicar una release de GitHub. El flujo prueba, analiza y compila el backend; instala dependencias, prueba y genera la compilación de producción del frontend; y construye los objetivos Docker `app` y `web` para `linux/amd64` y `linux/arm64`.

Los pushes a `develop` publican las imágenes en GHCR con la etiqueta mutable `develop`; los pushes a `main`, con la etiqueta mutable `rc`. Una release publicada genera etiquetas semver a partir de su tag (por ejemplo, `v1.0.0` genera `1.0.0`). Tras publicar las imágenes, un job independiente levanta la aplicación con Docker Compose y ejecuta `scripts/smoke.sh`: comprueba la API de salud, el HTML del frontend, la entrada QR de invitado, una subida PNG autenticada con su reintento idempotente, la galería, y el servido de medios por Caddy: `401` sin sesión, bytes idénticos, caché inmutable, `nosniff`, sin compresión, `?dl=1`, Range `206`, `Content-Type` de HEIC y `404` para rutas no publicadas. Para ejecutar esa prueba localmente desde la raíz del repositorio, usa el mismo script; necesita Docker Compose y `curl`, y al terminar elimina los contenedores y volúmenes del proyecto.

## 2.2 Configuración

**Regla:** lo estático o secreto va en variables de entorno; lo que se cambia con la aplicación en marcha, en SQLite.

Para ejecutar el backend, es **obligatorio** definir `BODA_DATA_DIR`, `BODA_EVENT_TOKEN`, `BODA_ADMIN_PASSWORD_HASH` y `BODA_SESSION_KEY`. Compose carga el archivo `.env` en el contenedor `app`; si falta alguna variable, `boda check` o `boda serve` falla al arrancar. Las demás variables son opcionales y usan los valores indicados cuando no se definen.

Parte del ejemplo completo en [`../.env.example`](../.env.example): cópialo a `.env` y reemplaza los valores de ejemplo antes de arrancar. Los secretos del ejemplo no son aptos para producción. En producción, `BODA_DATA_DIR` debe ser una raíz existente y escribible dentro del contenedor, montada desde el volumen persistente del servidor.

| Variable | Requerida | Ejemplo / valor predeterminado | Notas |
|---|---|---|---|
| `BODA_DOMAIN` | No | `:80` (Compose); `localhost` (Caddy) | Host que Caddy sirve. En producción, define el dominio público para habilitar HTTPS automático. |
| `BODA_ADDR` | No | `:8080` | Dirección de escucha del backend; se puede sobrescribir en `.env`. |
| `BODA_DATA_DIR` | **Sí** | `/srv/evento` | Raíz existente y escribible de **un único** montaje persistente. En Compose es el volumen `event-data`; `web` solo monta `originals/` y `derived/` en solo lectura. Si en producción se usa un directorio del host en lugar del volumen, debe pertenecer al UID `65532` (usuario `nonroot` de la imagen `app`). |
| `BODA_EVENT_TOKEN` | **Sí** | 32 bytes aleatorios codificados en base64url | Va en el QR. |
| `BODA_ADMIN_PASSWORD_HASH` | **Sí** | Hash bcrypt o argon2id | No es la contraseña en texto plano. |
| `BODA_SESSION_KEY` | **Sí** | 32 bytes aleatorios | Firma las cookies. |
| `BODA_MAX_PHOTO_BYTES` | No | `52428800` | Límite de foto: 50 MB. |
| `BODA_MAX_VIDEO_BYTES` | No | `1610612736` | Límite de vídeo: 1,5 GB. |
| `BODA_MAX_CONCURRENT_UPLOADS` | No | `30` | Por encima, 503 con `Retry-After`. |
| `BODA_UPLOAD_IDLE_TIMEOUT` | No | `90s` | Plazo de inactividad, no de duración total. |
| `BODA_DISK_MIN_FREE_BYTES` | No | `16106127360` | 15 GB; por debajo se cortan las subidas automáticamente. |
| `BODA_CLEANUP_INTERVAL` | No | `10m` | Intervalo de limpieza y conciliación. |
| `BODA_THUMB_FALLBACK` | No | `off` | Pasa a `on` en la fase 2. |
| `BODA_LOG_LEVEL` | No | `info` | Valores válidos: `debug`, `info`, `warn` o `error`. |

### Generar y aprovisionar el acceso QR

Genera el token del evento y la clave de sesión con un generador criptográfico del sistema. El primer comando produce los 32 bytes aleatorios del token en base64url sin padding; el segundo produce una clave de 32 caracteres ASCII (16 bytes aleatorios en hexadecimal), que satisface el requisito de `BODA_SESSION_KEY`:

```sh
BODA_EVENT_TOKEN=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')
BODA_SESSION_KEY=$(openssl rand -hex 16)
```

Aprovisiona ambos valores en el gestor de secretos o en el `.env` protegido del servidor, reemplazando entradas anteriores en vez de duplicarlas. No los guardes en Git, tickets, chats, capturas ni logs; limita el acceso al archivo de secretos. Configura el token antes de generar e imprimir el QR con la URL `https://<dominio>/e/<BODA_EVENT_TOKEN>`. Al entrar, la aplicación intercambia el token por una cookie de invitado y redirige a `/`; si el QR se filtra, genera un token nuevo, actualiza la configuración y vuelve a imprimirlo.

La cookie de invitado siempre lleva `Secure`, HttpOnly y SameSite=Lax: no existe una excepción insegura para desarrollo. En acceso local por HTTP (`http://localhost:8081`), los navegadores pueden rechazar o no reenviar una cookie `Secure`; prueba el flujo mediante HTTPS y no rebajes el atributo.

**En SQLite (tabla `settings`):** si las subidas están activas, el mensaje de bienvenida y los límites que el administrador quiera ajustar sin reiniciar.

**Validación al arrancar:** `boda check` y `boda serve` cargan la misma configuración y validan el almacenamiento antes de continuar. Si falta un secreto, si `BODA_DATA_DIR` no existe, si no se puede escribir en él o si `tmp/` y `originals/` están en sistemas de archivos distintos, **la aplicación no arranca**. La raíz debe existir previamente (por ejemplo, como montaje); la aplicación no la crea. Dentro de ella se preparan los directorios administrados con permisos privados y se comprueba que `tmp/` y `originals/` comparten sistema de archivos.

Ambos comandos abren `data/boda.db` con WAL, espera de bloqueo de 5 segundos, `synchronous=NORMAL` y claves foráneas. En la primera apertura se aplica la migración inicial, registrada en `PRAGMA user_version`, que crea las tablas `media` y `settings`. `boda check` también realiza esta inicialización y cierra la conexión al terminar, por lo que sirve para validar el despliegue antes de arrancar el servidor.

---

## 4.2 Pruebas mínimas antes de dar el diseño por bueno

**Dónde lanzarlas:** nunca desde el propio servidor. Puedes usar tu equipo o una de las instancias AMD micro de la capa gratuita como generador de carga. El ancho de banda se limita por conexión en la herramienta de subida, y se puede añadir pérdida de paquetes con `tc netem`.

**Qué medir en el servidor:** `docker stats` (memoria de cada proceso), `vmstat` (CPU y espera de disco), `iostat -x` (uso del disco) y `ls tmp/`.

| # | Prueba | Cómo | Criterio para darla por buena |
|---|---|---|---|
| **T1** | Matriz de dispositivos | Un iPhone reciente y un Android de gama media y otro de gama baja: foto HEIC y JPEG, vídeo 4K HEVC (iPhone) y MP4 (Android). Subir, ver en el otro dispositivo, reproducir y descargar | Todas las miniaturas se generan. Cada vídeo se reproduce o, si no, ofrece la descarga. Orientación correcta. El original descargado es idéntico byte a byte (mismo hash) |
| **T2** | Vídeo grande con 4G real (criterio de TUS) | 10 intentos de subir unos 300 MB desde el móvil con datos, a ser posible desde un sitio con mala cobertura | Menos del 30% de fallos, y los reintentos acaban completando la subida. Si no, se activa ADR-0005 y se pasa a TUS |
| **T3** | 25 subidas lentas a la vez | 25 subidas en paralelo de 200–500 MB, limitadas a 2–5 Mbps cada una | Memoria de la aplicación y de Caddy **estable** y por debajo de 1 GB, sin crecer con el tamaño de los archivos. CPU media por debajo del 50%. Ninguna escritura sin `fsync` perdida. La API de galería responde en menos de 500 ms (p95) durante la prueba |
| **T4** | Cortes y reintentos | Cortar el cliente al 50% (matar el proceso o cortar la red con `tc`) y reintentar con el mismo identificador. Otra vez, cortar la respuesta justo después de terminar | **Ningún duplicado**. `tmp/` vacío tras un ciclo de limpieza. El intento colgado no bloquea el reintento. Ningún registro se queda en `uploading` |
| **T5** | Carga mixta | T3 a la vez que 30 usuarios virtuales recorren la galería y 10 reproducen vídeos distintos saltando dentro de ellos | Todas las peticiones Range reciben 206 con el tamaño correcto. p95 de la API por debajo de 500 ms. Ningún error 5xx salvo los 503 deliberados por el límite de concurrencia |
| **T6** | Reiniciar a mitad de subida | Reiniciar el contenedor `app` durante T3 | La aplicación arranca sola. La conciliación deja un estado coherente. Los clientes reintentan y completan. Caddy sigue sirviendo la galería mientras tanto |
| **T7** | Disco lleno y ocultar | Subir el umbral de espacio libre por encima del espacio disponible. Ocultar un archivo y pedir su URL | Las subidas se desactivan automáticamente y las lecturas siguen. El archivo ocultado da 404 al instante y reaparece al volver a mostrarlo |
| **T8** | Entrega y recuperación | Hacer el `rsync` de `originals/` a tu equipo con la aplicación en marcha. Borrar `boda.db` en una copia y lanzar `boda reindex` | El `rsync` termina sin archivos parciales. El índice reconstruido lista todos los originales |
| **T9** | Ensayo general del despliegue | La semana anterior: ampliar el volumen, renovar el certificado, cambiar el token y reimprimir el QR, y probar el interruptor desde el móvil del administrador | Todo sale siguiendo `runbook.md`, sin improvisar |

**Orden recomendado:**
1. T1 y T2 con el primer vertical slice (acceso, subida de un archivo y persistencia). Son las pruebas que pueden cambiar el diseño.
2. T3 a T7 antes de construir la administración completa.
3. T8 y T9 en la semana previa al evento.

---

## Conciliación del almacenamiento

`boda serve` ejecuta una conciliación al arrancar y después cada `BODA_CLEANUP_INTERVAL` (10 minutos por defecto), según ADR-0009. Cada pasada:

- Borra los archivos parciales `tmp/*.part` abandonados. Nunca toca los de una subida en curso: se respeta el bloqueo por identificador que mantiene la subida.
- Publica en `derived/` las variantes que quedaron en `tmp/*.staged`, solo si su original está confirmado y visible. Las de originales ocultos o aún no confirmados siguen privadas.
- Mueve a `quarantine/` los archivos de `originals/` sin registro en SQLite. **Nunca los borra**, y si el nombre ya existe en `quarantine/` añade un sufijo numérico en vez de sobrescribir. Se respeta el original que se está confirmando en ese momento.

Cada pasada deja en el log una línea `reconciled upload storage` con los recuentos (`removed_partials`, `quarantined`, `promoted_derivatives`, `skipped_active`), y un error si algún archivo no se pudo tratar; el fallo de un archivo no detiene el resto de la pasada. Revisa `quarantine/` a mano antes del `rsync` final.

## Entrega a los novios

La entrega se hace al terminar mediante un `rsync` por SSH a un disco externo; la aplicación no interviene. El operador copia `originals/`, y copia `hidden/` solo si se pide. La copia de SQLite no forma parte de la entrega a los novios. Se puede hacer un `rsync` incremental la noche del evento para acortar la copia final. Los archivos no cambian y los temporales están aparte, por lo que el `rsync` es seguro con la aplicación en marcha. `quarantine/` se revisa a mano antes del `rsync` final.

**No se ha incluido en la fuente un comando `rsync` ejecutable** ni los valores de host, usuario, rutas de destino o puerto SSH; no se inventan aquí.

## Pendientes de operación indicados, pero no desarrollados en la fuente

- **Oracle Always Free:** la solicitud original requiere una advertencia sobre inactividad de CPU. La definición técnica aportada no especifica la política aplicable ni un procedimiento de mitigación; hay que completar y verificarlo antes del despliegue.
- **Ampliación del volumen:** la definición indica que el volumen de 200 GB se ampliará a más de 500 GB y que se debe preparar y probar la semana anterior. El árbol menciona `deploy/host/volume-resize.md` como destino para el procedimiento (consola, partición y sistema de archivos), pero esos pasos no están incluidos en el material entregado.

## Arranque local (sin Docker)

Antes de iniciar, exporta las variables obligatorias de la sección 2.2, incluidos los secretos, y configura `BODA_DATA_DIR` con una ruta existente y escribible. Por ejemplo, crea previamente la raíz de datos local; `boda` solo crea sus directorios administrados dentro de ella. No pongas secretos en la línea de comandos ni en el repositorio.

Valida la configuración y prepara el almacenamiento y la base de datos:

```sh
cd backend
BODA_ADDR=:8080 go run ./cmd/boda check
BODA_ADDR=:8080 go run ./cmd/boda serve
```

`check` y `serve` comparten la misma inicialización; el primero no deja abierta la conexión a SQLite. Para el frontend, ejecuta `cd frontend && npm run dev` (proxy `/api` → localhost:8080 por vite.config.ts).

- Docker Compose: `docker compose up --build` (web en :8081, app interno :8080)
- Smoke: `scripts/smoke.sh`
