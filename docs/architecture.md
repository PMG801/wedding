# Arquitectura técnica

## 0. Encaje del nuevo acuerdo

Doy por cerrada la entrega a los novios: al terminar, haces un `rsync` por SSH a un disco externo y la aplicación no interviene. Hay dos detalles que cuestan cero y facilitan ese `rsync`, y los incluyo en los ADRs:

1. **Nombres de archivo en orden cronológico:** `20260926T183012Z_{id}.{ext}`, con la hora del servidor al confirmar y un identificador aleatorio. Al ordenar por nombre, el disco sale en el orden de la boda. Aunque lleve la fecha delante, el nombre sigue siendo imposible de adivinar.
2. **`originals/` contiene solo lo visible y `hidden/` lo ocultado.** Por defecto le das a los novios `originals/`, y `hidden/` queda aparte por si lo quieren.

---

## 1. Stack tecnológico

### Resumen

| Pieza | Elección | Proceso en producción | Memoria aproximada en reposo |
|---|---|---|---|
| Proxy | **Caddy 2** (imagen oficial) | Sí | 30–60 MB |
| Backend | **Go** (versión estable actual) con la librería estándar | Sí, un único binario | 20–50 MB |
| Persistencia | **SQLite en modo WAL** con el driver `modernc.org/sqlite` (Go puro, sin C) | No, va dentro del binario | Incluida arriba |
| Miniaturas | **Canvas del navegador**; en la fase 2, `vipsthumbnail` y `ffmpeg` como procesos hijo | No | 0 (solo mientras se ejecutan) |
| Frontend | **Svelte 5 + TypeScript + Vite**, como SPA estática | No: son archivos estáticos | 0 |
| Despliegue | **Docker Compose con 2 servicios** e imágenes construidas con `buildx` | — | — |

En reposo son unos **100 MB de 12 GB**. La CPU solo se usa de verdad en TLS y, de forma ocasional, en el proceso hijo de la fase 2.

### 1.1 Proxy: Caddy 2

- **Por qué encaja:** es un binario en Go con imagen oficial para `linux/arm64`, usa poca memoria y cifra AES-GCM con las instrucciones de hardware de ARMv8, así que el TLS de 25 conexiones apenas cuenta.
- **Qué resuelve:**
  - HTTPS automático, con emisión y renovación de certificados.
  - Sirve archivos con peticiones Range, `ETag` y `Last-Modified` (su servidor de archivos se apoya en el mecanismo estándar de Go).
  - **Reenvía las peticiones sin guardarlas antes (streaming) por defecto.**
  - Reenvía las rutas de API (`/api/*`) y la entrada QR (`/e/*`) al backend.
  - La configuración cabe en unas 40 líneas.
- **Coste o complejidad:**
  - Hay que abrir los puertos 80 y 443 en **dos sitios**: la lista de seguridad de Oracle y el cortafuegos del propio sistema. Las imágenes de Ubuntu y Oracle Linux en OCI traen reglas de `iptables` restrictivas, y es un fallo clásico.
  - Solo activar la compresión en la aplicación estática, nunca en `/m/*`.
  - Fijar los protocolos en HTTP/1.1 y HTTP/2. HTTP/3 queda desactivado mientras no abramos UDP/443, para que los navegadores no intenten QUIC y tengan que volver atrás.
- **Descartado: Nginx.** Serviría, pero todo lo que necesitamos va en contra de su configuración por defecto: `client_max_body_size`, `proxy_request_buffering off`, `certbot` aparte y los plazos de espera. Cada uno de esos ajustes es un fallo silencioso si se olvida. **Traefik** también queda fuera: está pensado para enrutar contenedores, no para servir archivos.

### 1.2 Backend: Go con la librería estándar

- **Por qué encaja:** compila a un binario estático para ARM64 desde cualquier máquina, sin emulación. Las goroutines hacen que 30 subidas lentas cuesten unos pocos KB cada una, y la memoria por subida no depende del tamaño del archivo: se copia a disco con un buffer fijo.
- **Qué resuelve:**
  - Routing con métodos y rutas con variables, solo con la librería estándar.
  - Limitar el tamaño del cuerpo de la petición.
  - **Un plazo de inactividad por petición:** el plazo de lectura se renueva cada vez que llegan bytes, con el controlador de respuesta de `net/http`. Esto nos da justo el plazo por inactividad que acordamos, sin fijar un plazo total para toda la subida.
  - `fsync`, renombrado atómico y consulta del espacio libre del disco (`statfs`), todo en la librería estándar.
  - Logs estructurados con `slog`.
  - El frontend y las migraciones pueden ir embebidos en el binario si hiciera falta.
- **Coste o complejidad:** hay que escribir a mano el poco pegamento que daría un framework: middleware de autenticación, respuestas JSON y la cola interna de tareas. Son pocas piezas y se conocen bien.
- **Dependencias externas permitidas:** solo dos, el driver de SQLite y `golang.org/x/crypto` para calcular el hash de la contraseña del administrador.
- **Descartado: Node.js** (con Express o Fastify). No es incapaz, pero multer y busboy vuelcan a temporales por defecto, cuesta más memoria y hace falta Node en producción. **Python** (FastAPI) también queda fuera: el streaming de subidas y la concurrencia de un proceso ASGI exigen más cuidado para el mismo resultado. **Frameworks de Go** como Gin, Echo o Fiber: no aportan nada que la librería estándar no cubra, y Fiber ni siquiera usa `net/http`.

### 1.3 Persistencia: SQLite con el driver `modernc.org/sqlite`

- **Por qué encaja:** no hay proceso aparte, ocupa unos pocos MB y la carga es de decenas de escrituras por minuto como máximo.
- **Qué resuelve:** metadatos, estados, cursores de la galería, cola de tareas, configuración que se puede cambiar en marcha (el interruptor) y credenciales. La copia en caliente se hace con `VACUUM INTO`.
- **Por qué este driver:** está escrito en Go puro. Así se mantiene `CGO_ENABLED=0`, la compilación cruzada es trivial y la imagen final puede ser mínima. Es algo más lento que el driver en C, pero con esta carga da igual.
- **Coste o complejidad:** las migraciones se hacen sin librería, con archivos SQL embebidos numerados y el campo `PRAGMA user_version`. Hay que configurar el modo WAL, `busy_timeout`, `synchronous=NORMAL` y `foreign_keys` al abrir la base de datos.
- **Descartado: PostgreSQL**, porque es otro servicio con su propia copia de seguridad y sus actualizaciones sin ninguna ganancia. **`mattn/go-sqlite3`** tampoco: exige compilar código C, lo que complica `buildx` y la compilación cruzada. **Un ORM** (GORM o similar): con 3 o 4 tablas, SQL directo es más claro.

### 1.4 Procesamiento de miniaturas

| Fase | Herramienta | Uso |
|---|---|---|
| **1 (lanzamiento)** | Canvas del navegador, `createImageBitmap` y codificación a **JPEG** | La miniatura y la versión de detalle se generan en el móvil; la portada del vídeo, con un `<video>` fuera de pantalla capturado en un canvas |
| **2 (respaldo, opcional)** | `vipsthumbnail` (libvips, con libheif) y `ffmpeg`, lanzados como **proceso hijo**, uno cada vez, con prioridad baja y límite de tiempo | Solo para lo que llegue sin miniatura, casi siempre HEIC de móviles Samsung o casos en que el canvas falló |

- **Por qué JPEG y no WebP:** JPEG se codifica en todos los navegadores. Que Safari codifique WebP desde el canvas no es algo con lo que contar.
- **Por qué libvips y no ImageMagick:** libvips procesa la imagen por trozos (streaming) y usa una fracción de la memoria. ImageMagick puede consumir cientos de MB con una foto de 48 MP.
- **Por qué proceso hijo y no una librería dentro de la aplicación** (govips, por ejemplo): un fallo del decodificador no se lleva por delante la aplicación, y no hace falta compilar código C.
- **Descartado: decodificar HEIC en el navegador con WebAssembly** (libheif-js). Añade varios MB al bundle, consume mucha memoria en móviles de gama baja, y el iPhone ya decodifica HEIC de forma nativa.
- **Coste:** la fase 2 obliga a usar una imagen basada en Debian en lugar de una mínima (unos 150–250 MB más). **Hay que comprobar** que el paquete de libheif de esa versión incluye el decodificador HEVC (libde265). En algunas versiones de Debian va como plugin aparte.

### 1.5 Frontend: Svelte 5, TypeScript y Vite

- **Por qué encaja:** el resultado son archivos estáticos que sirve Caddy, sin proceso en el servidor. Svelte no arrastra apenas código de framework al navegador, lo que importa en móviles con 4G.
- **Qué resuelve:** cinco pantallas (bienvenida, subida, galería, detalle y administración) y una cola de subidas con estado reactivo: progreso, reintentos y errores por archivo.
- **Routing:** cinco rutas no justifican SvelteKit. Basta con un router mínimo de cliente o con rutas por hash.
- **Coste:** hace falta Node **solo para compilar**, en la etapa de build de Docker o en local.
- **Descartado: React** (con Vite): más código de framework y más piezas que elegir (router, estado) sin ninguna ventaja aquí. **Next.js o SvelteKit con servidor:** meten Node en producción. **htmx o JavaScript sin framework:** la cola de subidas con su estado hace que en poco tiempo acabes escribiendo tu propio framework.

### 1.6 Piezas descartadas en todo el stack

- Prometheus o Grafana: para observar basta con `docker stats`, `vmstat`, `iostat` y los logs.
- Redis, colas externas, MinIO o S3.
- Kubernetes.
- Límites de peticiones en el proxy: exigirían compilar Caddy con un plugin. El único límite que hace falta, en el login del administrador, lo aplica la aplicación.
- WebSockets.

---

## 2.3 Build y despliegue

**Compose, con 2 servicios:**
- `app`: monta `/srv/evento` completo, **como un único montaje**.
  > **Trampa:** si se montan `tmp/`, `originals/` y `hidden/` como montajes separados, el renombrado entre ellos falla (error `EXDEV`) y se rompe la confirmación.
- `web` (Caddy):
  - Monta en **solo lectura** `originals/` y `derived/`, y nada más.
  - Guarda sus certificados en un volumen propio, fuera de `/srv/evento`.
- Rotación de logs de Docker (`max-size` y `max-file`) en ambos servicios, porque comparten disco con los datos.
- `restart: unless-stopped` y un healthcheck que llama a `boda check`.

**Dockerfile multietapa con buildx:**
1. **Etapa `frontend`:** Node en la plataforma del equipo que compila (`$BUILDPLATFORM`) genera `dist/`. El resultado es igual en cualquier arquitectura, así que no hace falta emulación.
2. **Etapa `backend`:** Go en `$BUILDPLATFORM`, con `GOOS=linux`, `GOARCH=$TARGETARCH` y `CGO_ENABLED=0`. Es compilación cruzada nativa, **sin QEMU**, y tarda segundos.
3. **Destino `app`:**
   - **Fase 1:** una imagen mínima (distroless o similar) solo con el binario.
   - **Fase 2:** Debian slim con `libvips-tools` y `ffmpeg`. Solo aquí hay paquetes que dependen de la arquitectura; se instalan en la etapa de la plataforma de destino, y bajo emulación sigue siendo razonable.
4. **Destino `web`:** la imagen oficial de Caddy más `dist/` y el `Caddyfile`.

**Plataformas:**
- `linux/arm64` para producción.
- `linux/amd64` opcional, para probar en local desde un portátil x86.

**Distribución**, elige una:
- **(a)** Subir las imágenes a un registro (por ejemplo GHCR) y hacer `pull` en el servidor.
- **(b)** Sin registro: `docker save`, enviarlo por SSH y `docker load`.
- **(c)** Compilar directamente en el propio servidor ARM. Es válido, porque la compilación pesa poco.

Recomiendo **(a)** si ya tienes GHCR y **(c)** si quieres cero infraestructura adicional.

**Versionado:** se etiquetan las imágenes con el hash del commit y se congela una versión etiquetada antes del evento. **Nada de desplegar el día de la boda.**
