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

### Aprovisionamiento del host (una vez)

Hazlo antes del primer despliegue. Requiere SSH con `sudo`.

1. **Consola de OCI:** en la security list (o NSG) de la subred de la instancia, añade un ingreso TCP **80** y **443** desde `0.0.0.0/0`. El cortafuegos del host no basta por sí solo: las imágenes de Ubuntu y Oracle Linux en OCI traen reglas de `iptables` restrictivas.
2. **Copia de archivos:**

```sh
sudo install -d -o "$USER" -g "$USER" /srv/boda
scp deploy/server/{compose.yaml,deploy.sh,bootstrap.sh,provision-host.sh} <servidor>:/srv/boda/
```

3. **Provisión del host.** Es opcional y repetible: `--dry-run` solo imprime el plan.

```sh
ssh <servidor>
cd /srv/boda
sudo ./provision-host.sh --dry-run
sudo ./provision-host.sh
```

Cada paso comprueba antes el estado actual y no repite lo que ya está hecho:

- Instala Docker Engine y el plugin de Compose si faltan. En Ubuntu usa el script oficial `get.docker.com`. En Oracle Linux hay que instalar Docker antes desde el repositorio de Docker; el script se detiene si no lo encuentra.
- Habilita y arranca `docker`.
- Abre TCP 80 y 443 en `INPUT` antes de la regla `REJECT` y guarda las reglas en `/etc/iptables/rules.v4` (con `netfilter-persistent`; en Oracle Linux, en `/etc/sysconfig/iptables`). Si `firewalld` está activo, el script se detiene y muestra los comandos equivalentes.
- Crea `/swapfile` de 2 GB y lo añade a `/etc/fstab`, solo si el host no tiene swap activa.
- Fija `vm.swappiness = 10` en `/etc/sysctl.d/99-boda.conf`.
- Activa las actualizaciones de seguridad automáticas (`unattended-upgrades`; en Oracle Linux, `dnf-automatic`) y la sincronización horaria.

Si el volumen de bloque de 200 GB va a ser la raíz de datos, adjúntalo en la consola de OCI, averigua su ruta con `lsblk` y pásala al script. Se monta en `/srv/boda/data` por UUID en `/etc/fstab`, con `nofail,_netdev`:

```sh
sudo ./provision-host.sh --data-device /dev/sdb
```

- Si el dispositivo no tiene sistema de archivos, el script se detiene. Solo lo formatea (ext4) si añades `--format-data-device`, **lo que borra todo el dispositivo**. No lo uses en un dispositivo con datos.
- Ejecuta este paso **antes** de `bootstrap.sh`: el montaje se niega si `/srv/boda/data` ya contiene archivos.
- `deploy.sh` se niega a arrancar si `/srv/boda/data` figura en `/etc/fstab` y no está montado, para no escribir en el disco del sistema.

4. **Docker sin `sudo`:** `bootstrap.sh` y `deploy.sh` llaman a Docker sin `sudo`, así que el operador debe estar en el grupo `docker`: `sudo usermod -aG docker <operador>`, y volver a entrar por SSH. Ese grupo equivale a root en el host; es una decisión del operador.
5. **Directorios y secretos:**

```sh
cd /srv/boda && ./bootstrap.sh
```

`bootstrap.sh` crea `data/` (propiedad del UID `65532`, modo `0700`) y genera un `.env` con permisos `600` y secretos aleatorios, más `.admin-password`. Es idempotente: nunca sobrescribe un `.env` existente. Ninguno de los dos archivos está en Git.

Para desplegar o actualizar: `cd /srv/boda && ./deploy.sh` (hace `docker compose pull` y `up -d`).

### Registros, memoria y usuario de Caddy

- **Registros:** ambos servicios usan `json-file` con `max-size: 10m` y `max-file: 3`: como mucho 30 MB por contenedor. Comparten disco con los datos.
- **Memoria:** `app` tiene un límite de 1 GiB y `GOMEMLIMIT=512MiB`, un objetivo blando que hace trabajar más al recolector de Go antes de llegar al límite. `web` tiene 512 MiB. Para medirlo durante una carga: `docker stats --no-stream`.
- **Descriptores:** `nofile` 65536 en ambos servicios.
- **Healthcheck de `web`:** consulta la API de administración de Caddy dentro del contenedor (`127.0.0.1:2019/config/`). `docker compose ps` debe mostrar `healthy`.
- **Caddy sin root:** corre como UID `65532`, el mismo que `app`. Conserva `cap_net_bind_service` para escuchar en 80 y 443, y sus volúmenes `caddy-data` y `caddy-config` pertenecen a ese UID. Un volumen creado por una versión anterior (con root) hay que corregirlo una vez, con los servicios parados:

```sh
cd /srv/boda && docker compose down
docker run --rm --user root --entrypoint sh \
  -v boda_caddy-data:/data -v boda_caddy-config:/config \
  ghcr.io/pmg801/boda-web:rc -c 'chown -R 65532:65532 /data /config'
docker compose up -d
```

`docker compose down` sin `-v` conserva los volúmenes y los certificados.

Por defecto la web solo escucha en loopback del servidor: HTTP en `127.0.0.1:8081` (`BODA_HTTP_PORT`) y HTTPS en `127.0.0.1:8443` (`BODA_HTTPS_PORT`, `BODA_HTTPS_BIND`). Se accede con un túnel SSH, porque la cookie `Secure` solo se acepta en HTTPS o en `localhost`:

```sh
ssh -L 8081:127.0.0.1:8081 <servidor>
# QR de prueba: http://localhost:8081/e/<BODA_EVENT_TOKEN>
```

Para publicar solo con la IP (fase de pruebas, sin dominio): define en `.env` `BODA_DOMAIN=<IP pública>`, `BODA_HTTPS_BIND=0.0.0.0` y `BODA_HTTPS_PORT=443`. Caddy emite entonces un certificado de su CA local para esa IP (válido unas 12 horas y renovado automáticamente), así que cada dispositivo muestra un aviso de certificado no confiable que hay que aceptar; sin HTTPS la cookie `Secure` no se guarda. Hay que abrir TCP 80 y 443, tanto en el firewall del host (lo hace `provision-host.sh`) como en la security list de la nube. Para que el 80 sea público en esta fase, define además `BODA_HTTP_BIND=0.0.0.0` y `BODA_HTTP_PORT=80`; pero con la IP en `BODA_DOMAIN` el 80 solo redirige a HTTPS; no expongas el 80 sin definir antes `BODA_DOMAIN`, porque sin él la web responde en claro. Con dominio real, basta con poner `BODA_DOMAIN=<dominio>` y Caddy obtendrá un certificado de Let's Encrypt (en ese caso sí necesita el 80 o el 443 accesibles para la validación).

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
| `BODA_MAX_DERIVATIVE_BYTES` | No | `8388608` | Límite de cada miniatura o detalle JPEG: 8 MiB. Por encima, `413 derivative_too_large` sin leer el cuerpo. |
| `BODA_MAX_CONCURRENT_UPLOADS` | No | `30` | Por encima, 503 con `Retry-After`. |
| `BODA_UPLOAD_IDLE_TIMEOUT` | No | `90s` | Plazo de inactividad, no de duración total. |
| `BODA_DISK_MIN_FREE_BYTES` | No | `16106127360` | 15 GB; por debajo se cortan las subidas automáticamente. |
| `BODA_CLEANUP_INTERVAL` | No | `10m` | Intervalo de limpieza y conciliación. |
| `BODA_STAGED_DERIVATIVE_TTL` | No | `24h` | Tiempo que se guardan miniaturas cuyo original nunca se confirma; después la conciliación las borra. |
| `BODA_BACKUP_INTERVAL` | No | `24h` | Intervalo de la copia de SQLite en `backup/` (ver «Copias de seguridad de SQLite»). |
| `BODA_BACKUP_KEEP` | No | `7` | Número de copias de SQLite que se conservan; se borran las más antiguas. |
| `BODA_THUMB_FALLBACK` | No | `off` | Pasa a `on` en la fase 2. |
| `BODA_LOG_LEVEL` | No | `info` | Valores válidos: `debug`, `info`, `warn` o `error`. |

`GOMEMLIMIT=512MiB` no es una variable `BODA_*`: Compose la fija en el contenedor `app` como límite blando del recolector de Go, por debajo de su límite de memoria de 1 GiB.

### Generar y aprovisionar el acceso QR

Genera el token del evento y la clave de sesión con un generador criptográfico del sistema. El primer comando produce los 32 bytes aleatorios del token en base64url sin padding; el segundo produce una clave de 32 caracteres ASCII (16 bytes aleatorios en hexadecimal), que satisface el requisito de `BODA_SESSION_KEY`:

```sh
BODA_EVENT_TOKEN=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')
BODA_SESSION_KEY=$(openssl rand -hex 16)
```

Aprovisiona ambos valores en el gestor de secretos o en el `.env` protegido del servidor, reemplazando entradas anteriores en vez de duplicarlas. No los guardes en Git, tickets, chats, capturas ni logs; limita el acceso al archivo de secretos. Configura el token antes de generar e imprimir el QR con la URL `https://<dominio>/e/<BODA_EVENT_TOKEN>`. Al entrar, la aplicación intercambia el token por una cookie de invitado y redirige a `/`; si el QR se filtra, genera un token nuevo, actualiza la configuración y vuelve a imprimirlo.

La cookie de invitado siempre lleva `Secure`, HttpOnly y SameSite=Lax: no existe una excepción insegura para desarrollo. En acceso local por HTTP (`http://localhost:8081`), los navegadores pueden rechazar o no reenviar una cookie `Secure`; prueba el flujo mediante HTTPS y no rebajes el atributo.

**En SQLite (tabla `settings`):** si las subidas están activas, el mensaje de bienvenida y los límites que el administrador quiera ajustar sin reiniciar.

**Validación al arrancar:** `boda check` y `boda serve` cargan la misma configuración y validan el almacenamiento antes de continuar. Si falta un secreto, si `BODA_DATA_DIR` no existe, si no se puede escribir en él o si `tmp/`, `originals/`, `derived/` y `hidden/` no están en el mismo sistema de archivos, **la aplicación no arranca**. La raíz debe existir previamente (por ejemplo, como montaje); la aplicación no la crea. Dentro de ella se preparan los directorios administrados con permisos privados y se comprueba que `tmp/`, `originals/`, `derived/` y `hidden/` comparten sistema de archivos.

Ambos comandos abren `data/boda.db` con WAL, espera de bloqueo de 5 segundos, `synchronous=FULL`, un `journal_size_limit` de 64 MiB para truncar el WAL tras los puntos de control, y claves foráneas. `synchronous=FULL` hace que cada confirmación espere al disco: una caída de energía no pierde subidas ya confirmadas aunque cueste algo más de latencia. En la primera apertura se aplica la migración inicial, registrada en `PRAGMA user_version`, que crea las tablas `media` y `settings`. `boda check` también realiza esta inicialización y cierra la conexión al terminar, por lo que sirve para validar el despliegue antes de arrancar el servidor.

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
| **T9** | Ensayo general del despliegue | La semana anterior: ampliar el volumen, renovar el certificado, cambiar el token y reimprimir el QR, probar el interruptor desde el móvil del administrador y recorrer una galería de 500 o más medios en un móvil real | Todo sale siguiendo `runbook.md`, sin improvisar. La galería no se queda sin memoria ni salta al hacer scroll |

**Orden recomendado:**
1. T1 y T2 con el primer vertical slice (acceso, subida de un archivo y persistencia). Son las pruebas que pueden cambiar el diseño.
2. T3 a T7 antes de construir la administración completa.
3. T8 y T9 en la semana previa al evento.

---

## Pruebas de carga

`scripts/load.sh` lanza `scripts/load/upload.js` con k6 (binario local o, si no existe, la imagen `grafana/k6` por Docker). Genera fotos sintéticas (firma JPEG + ceros), así que no hace falta descargar imágenes. Cada invitado virtual entra por el QR, sube una foto, la reenvía (debe responder 200) y lee la galería.

```bash
set -a; . ./.env; set +a
LOAD_BASE_URL=http://localhost:8081 LOAD_EVENT_TOKEN="$BODA_EVENT_TOKEN" \
LOAD_PHOTO_BYTES=5242880 LOAD_MAX_TOTAL_BYTES=1073741824 LOAD_VUS=20 \
scripts/load.sh
```

Los límites los pones tú con variables de entorno y la prueba se detiene en el primero que se alcance:

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `LOAD_MAX_TOTAL_BYTES` | 100 MiB | Presupuesto duro de bytes enviados (incluye el reenvío). |
| `LOAD_PHOTO_BYTES` | 2 MiB | Tamaño de cada foto. |
| `LOAD_VUS` | 5 | Invitados concurrentes. |
| `LOAD_MAX_DURATION` | 5m | Tope de tiempo. |
| `LOAD_MAX_ERROR_RATE` | 0.05 | Aborta si los errores inesperados superan esta tasa. |
| `LOAD_STOP_ON_STATUS` | 507 | Aborta cuando el servidor responde con alguno de estos códigos. |
| `LOAD_IDEMPOTENT_RETRY` | true | Reenvía cada foto esperando 200. |
| `LOAD_GALLERY_READS` | 1 | Lecturas de galería por iteración. |

Para provocar los límites del servidor sin agotar la máquina, baja sus variables `BODA_*` en el entorno de pruebas (por ejemplo `BODA_DISK_MIN_FREE_BYTES` para obtener 507, `BODA_MAX_PHOTO_BYTES` para 413 o `BODA_MAX_CONCURRENT_UPLOADS` para 503). Los códigos 408, 413, 503 y 507 se cuentan aparte (`limit_hits`) y no se consideran fallos. Con HTTPS autofirmado añade `LOAD_INSECURE=1`. Las fotos subidas quedan en el volumen: bórralo al terminar.

## Pruebas de carga T3 y T5 en el A1 de QA

Estas pruebas reproducen en el servidor de QA las condiciones del evento: 25 subidas de vídeo lentas a la vez (T3) y, en paralelo, lecturas de galería y de rangos de vídeo (T5). Corren desde un equipo generador, nunca en el servidor.

- **Generador:** `scripts/load/slow-upload.sh` (T3) y `scripts/load/readers.sh` (T5). Requieren bash, curl y Docker o k6.
- **Servidor:** `scripts/load/sample-server.sh`, solo lectura; necesita sudo para leer `data/tmp`.
- **Comunes:** `LOAD_BASE_URL` y `LOAD_EVENT_TOKEN` son obligatorias y no tienen valor por defecto. `LOAD_RUN_DIR` es la carpeta de resultados. Los resultados contienen métricas del servidor: no los subas a Git.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `LOAD_SLOW_COUNT` | 25 | Número de subidas lentas (T3). |
| `LOAD_SLOW_BYTES` | 200M | Tamaño de cada vídeo; admite K, M y G. |
| `LOAD_SLOW_MBPS` | 3 | Ancho de banda por conexión, en Mbit/s. |
| `LOAD_SLOW_RETRY` | 0 | Si vale 1, reenvía cada subida con el mismo identificador (reenvío idempotente). |
| `LOAD_MAX_TOTAL_BYTES` | 12G | Presupuesto total de bytes enviados. |
| `LOAD_MAX_DURATION` | 5400 s | Tope de tiempo de la tanda T3. |
| `LOAD_STOP_ON_STATUS` | 507 | Detiene la tanda al recibir este código. |
| `LOAD_DURATION` | 10m | Duración de las lecturas T5. |
| `LOAD_GALLERY_VUS` | 30 | Usuarios virtuales que recorren la galería. |
| `LOAD_RANGE_VUS` | 10 | Usuarios virtuales que reproducen vídeos con saltos (Range). |
| `LOAD_RANGE_BYTES` | 1 MiB | Tamaño de cada petición Range. |
| `LOAD_GALLERY_P95_MS` | 500 | Umbral de p95 de la galería. |
| `LOAD_MAX_ERROR_RATE` | 0.05 | Tasa máxima de errores antes de abortar T5. |

**Procedimiento:**

1. Si el QA está vacío, siembra vídeos con una tanda de T3.
2. En el servidor, arranca el muestreo durante una hora: `sudo LOAD_SAMPLE_PROJECT=boda scripts/load/sample-server.sh --data-dir /srv/boda/data --interval 15 --duration 3600 --out /srv/boda/loadtest-samples/AAAA-MM-DD`.
3. En el generador, define el destino y la carpeta de resultados y lanza T3 y T5 a la vez:

   ```sh
   export LOAD_BASE_URL=https://qa.ejemplo.org LOAD_EVENT_TOKEN='…' LOAD_RUN_DIR=loadtest/results/AAAA-MM-DD
   LOAD_SLOW_COUNT=25 LOAD_SLOW_BYTES=200M LOAD_SLOW_MBPS=3 scripts/load/slow-upload.sh &
   LOAD_DURATION=12m scripts/load/readers.sh &
   wait
   ```

   Con 200 MB a 3 Mbit/s, cada subida tarda unos 9 minutos.
4. Copia `samples.csv` e `iostat.log` a `LOAD_RUN_DIR` y borra las subidas de prueba del QA.

**Criterios de lectura:**

- `summary.txt`: al menos 25 filas `put 201`. Explica cualquier 507, 503 o 5xx. La línea `STOP flag was set` indica un rechazo por espacio o por límite.
- `readers.log`: los umbrales `bad_responses` (0 %) y `http_req_duration{name:gallery}` deben aparecer en ✓, y los rangos deben responder 206 con un `Content-Range` exacto.
- `samples.csv`: `app_mem_mib + web_mem_mib` por debajo de 1024 MiB y sin crecer de forma continua; la media de `vm_us + vm_sy` por debajo de 50; `data_avail_mib` no debe bajar de `BODA_DISK_MIN_FREE_BYTES`; `tmp_entries` debe volver a 0.
- La p95 de la galería debe quedar por debajo de 500 ms.
- `iostat.log` requiere el paquete `sysstat` en el servidor.

**Reenvío tras la corrección T10:** con la corrección de los reintentos de subidas ya confirmadas, `LOAD_SLOW_RETRY=1` debe devolver 200 en el reenvío a través de Caddy. Si no lo hace, anótalo como fallo.

## Interruptor de subidas y administración desde el móvil

`/admin` es la página de administración para el móvil, en español. Permite:

- Iniciar sesión con la contraseña cuyo hash está en `BODA_ADMIN_PASSWORD_HASH`. El hash se genera con bcrypt (`htpasswd -nBC 12 ""`; de la salida, que tiene la forma `:<hash>`, usa solo el hash) o con argon2id. Los `$` del hash van entre comillas simples en `.env` y se escriben como `$$` en los archivos de Compose.
- Pausar o reanudar las subidas con un interruptor. Con las subidas en pausa, `PUT` y los derivados responden `503 uploads_paused`; la galería sigue funcionando.
- Ver el aviso de pausa automática por disco y el espacio libre y reservado, en GB.
- Ver la lista **Ocultas** con sus miniaturas y volver a mostrar cualquiera con **Mostrar**.

### Ocultar contenido

Con la sesión de administrador abierta en el mismo navegador, y tras entrar también por el QR, el visor de cada foto o vídeo de la galería muestra el botón **Ocultar**. Pide confirmación y, al aceptar, el medio desaparece de la galería y su URL da 404 al instante: el original pasa a `hidden/` y sus miniaturas a `tmp/`. Se restaura desde **Ocultas** en `/admin`.

**No se borra nada desde la aplicación**, ni siquiera como administrador: no hay botón ni endpoint de borrado. Ocultar siempre es reversible, y lo oculto sigue en `hidden/` para la entrega si se pide.

Notas de operación:

- La sesión de administrador dura 12 horas. Cerrar sesión no la revoca en el servidor.
- Tras los fallos de contraseña, el retraso progresivo del login vive en memoria: se reinicia al reiniciar la aplicación.
- Rotar `BODA_SESSION_KEY` invalida a la vez las sesiones de invitados y de administrador.
- Prueba el interruptor desde el móvil del administrador en el ensayo general (T9).

## «Me gusta» y «Más gustadas»

Cada invitado puede marcar fotos y vídeos con un corazón, en la cuadrícula y en el visor. «Más gustadas» muestra los 50 medios visibles con más «me gusta» (al menos uno); «Recientes» sigue siendo la galería cronológica (ADR-0012).

- Los «me gusta» y su recuento se guardan en SQLite (`media_likes` y `media.like_count`): sobreviven a reinicios y entran en la copia de seguridad de la base. No hay ninguna caché que reiniciar.
- Cada cookie de invitado es un votante. El mismo navegador no puede votar dos veces el mismo medio, ni volviendo a escanear el QR; otro navegador, otro dispositivo, borrar la caché o la caducidad de la cookie (30 días) sí permiten votar de nuevo. Se acepta.
- Rotar `BODA_SESSION_KEY` crea identidades nuevas para todos: los invitados podrían volver a votar lo que ya votaron. Evítalo durante el evento.
- Ocultar un medio lo quita del ranking y bloquea sus «me gusta», pero no los borra: al mostrarlo de nuevo recupera su recuento.

## Comportamiento de la subida en el cliente

- Si una subida no recibe progreso durante 60 segundos, el cliente la aborta y la reintenta. No hay plazo total: un vídeo grande puede tardar lo que haga falta mientras siga avanzando.
- Hasta 5 reintentos, con espera exponencial de 1 s × 2^n y jitter (entre la mitad y el total). La espera nunca baja de `Retry-After` si el servidor lo envía.
- `503 uploads_paused`: no se reintenta automáticamente. El invitado ve «Las subidas están en pausa» y reintenta a mano.
- `507 insufficient_storage`: no se reintenta.
- Galería: las miniaturas nunca muestran el original. Si falta la miniatura, se muestra «Vista previa no disponible». El original solo aparece en el detalle, y solo para JPEG y PNG.

## Vídeos

Los invitados pueden subir vídeo MP4, M4V y QuickTime MOV (se detectan por la firma `ftyp`, no por la extensión) de hasta `BODA_MAX_VIDEO_BYTES` (1,5 GB). El servidor no transcodifica: guarda y sirve el original con Range (`206`). El navegador del invitado captura un fotograma como portada (miniatura y detalle) y, si no puede decodificar el vídeo (por ejemplo HEVC fuera de Safari), sube solo el original y la galería muestra un marcador con el icono de reproducción. Si el navegador tampoco puede reproducirlo, el detalle ofrece la descarga del original.

En el cliente, un vídeo se sube solo: espera a que terminen las subidas en curso y no empieza ninguna otra hasta que acaba. Los límites de espacio libre y de concurrencia son los mismos que para las fotos. Las pruebas T1 y T2 de la matriz (dispositivos reales y 4G) siguen siendo manuales y deben hacerse antes de imprimir el QR; `scripts/smoke.sh` cubre subida, reintento idempotente, tipo de contenido y Range de un MP4 sintético.

## Conciliación del almacenamiento

`boda serve` ejecuta una conciliación al arrancar y después cada `BODA_CLEANUP_INTERVAL` (10 minutos por defecto), según ADR-0009. Cada pasada:

- Primero hace que la ubicación de los archivos siga la visibilidad de SQLite: si una ocultación o una restauración se interrumpió, mueve el original de un medio oculto a `hidden/` y sus derivados a `tmp/*.staged`, o devuelve a `originals/` el de un medio visible que quedó en `hidden/`. Los archivos de `hidden/` sin registro no se tocan ni van a `quarantine/`.
- Borra los archivos parciales `tmp/*.part` abandonados. Nunca toca los de una subida en curso: se respeta el bloqueo por identificador que mantiene la subida.
- Publica en `derived/` las variantes que quedaron en `tmp/*.staged`, solo si su original está confirmado y visible. Las de originales ocultos o aún no confirmados siguen privadas.
- Borra las variantes `tmp/*.staged` cuyo original nunca se confirmó, una vez pasado `BODA_STAGED_DERIVATIVE_TTL` (24 horas por defecto). Así `tmp/` no crece sin límite con miniaturas huérfanas.
- Mueve a `quarantine/` los archivos de `originals/` sin registro en SQLite. **Nunca los borra**, y si el nombre ya existe en `quarantine/` añade un sufijo numérico en vez de sobrescribir. Se respeta el original que se está confirmando en ese momento.

Cada pasada deja en el log una línea `reconciled upload storage` con los recuentos (`removed_partials`, `quarantined`, `promoted_derivatives`, `removed_staged`, `skipped_active`, `visibility_repairs`), y un error si algún archivo no se pudo tratar; el fallo de un archivo no detiene el resto de la pasada. Revisa `quarantine/` a mano antes del `rsync` final.

## Copias de seguridad de SQLite

`boda serve` escribe una copia de `boda.db` con `VACUUM INTO` en `<BODA_DATA_DIR>/backup/` cada `BODA_BACKUP_INTERVAL` (24 horas por defecto). La primera copia se hace al arrancar, en segundo plano, sin retrasar el servicio. Se conservan solo las `BODA_BACKUP_KEEP` (7) más recientes.

- Los archivos se llaman `boda-AAAAMMDDTHHMMSSZ.db` (hora UTC). Se escriben con un nombre temporal y se renombran al final, así que un corte nunca deja una copia truncada que parezca válida.
- La copia se hace con la aplicación en marcha y no interrumpe las subidas.
- Si una copia falla, se registra como `backup sqlite database` con el error y el servidor sigue funcionando.
- Las copias están en el mismo volumen que los originales. No sustituyen a una copia fuera del servidor.

**Restaurar una copia** (operación excepcional; hazla con la aplicación parada):

1. Para la aplicación: `docker compose stop app` (o detén `boda serve` en local). Antes de restaurar, guarda una copia del `boda.db` actual.
2. Elige la copia más reciente anterior al problema: `ls -l <BODA_DATA_DIR>/backup/`.
3. Sustituye la base de datos y elimina los archivos WAL. Como la copia la crea la aplicación, el archivo restaurado debe seguir perteneciendo al UID `65532`:

   ```sh
   cp <BODA_DATA_DIR>/backup/boda-AAAAMMDDTHHMMSSZ.db <BODA_DATA_DIR>/data/boda.db
   rm -f <BODA_DATA_DIR>/data/boda.db-wal <BODA_DATA_DIR>/data/boda.db-shm
   chown 65532:65532 <BODA_DATA_DIR>/data/boda.db
   ```

4. Arranca la aplicación: `docker compose start app`. Comprueba `/api/health` y la galería.

Las subidas posteriores a la copia siguen en `originals/` sin registro en SQLite. La conciliación no las borra: las mueve a `quarantine/`, donde conviene revisarlas a mano antes de la entrega.

## Entrega a los novios

La entrega se hace al terminar mediante un `rsync` por SSH a un disco externo; la aplicación no interviene. El operador copia `originals/`, y copia `hidden/` solo si se pide. La copia de SQLite no forma parte de la entrega a los novios. Se puede hacer un `rsync` incremental la noche del evento para acortar la copia final. Los archivos no cambian y los temporales están aparte, por lo que el `rsync` es seguro con la aplicación en marcha. `quarantine/` se revisa a mano antes del `rsync` final.

**Comando de entrega.** Se ejecuta desde tu equipo o el disco de destino, no en el servidor. `data/` pertenece al UID `65532` y tiene modo `0700`, así que el `rsync` del servidor usa `sudo`:

```sh
rsync -a --info=progress2 --rsync-path='sudo rsync' \
  <operador>@<servidor>:/srv/boda/data/originals/ <disco>/originals/
```

- Sin `--partial`: un archivo interrumpido se descarta en vez de quedar con su nombre final. Para reanudar, vuelve a lanzar el mismo comando.
- Sin `--delete`: el `rsync` nunca borra nada del destino.
- Para copiar también `hidden/`, repite el comando con `data/hidden/` y `<disco>/hidden/`.
- `sudo` no puede pedir contraseña en una sesión SSH sin terminal, así que en el servidor hace falta una regla limitada, creada una vez con `sudo visudo -f /etc/sudoers.d/boda-rsync`:

```text
<operador> ALL=(root) NOPASSWD: /usr/bin/rsync --server --sender *
```

  Comprueba la sintaxis con `sudo visudo -cf /etc/sudoers.d/boda-rsync`. La regla solo permite el modo de envío de `rsync`, pero como root ese modo lee cualquier archivo del servidor: trátala como una credencial de administración.

## Pendientes de operación (gestionados a mano por el operador, L2)

El repositorio no los automatiza. El operador los prepara y prueba antes del evento, según la decisión L2:

- **Oracle Always Free (reclamación por inactividad):** la política de inactividad de la instancia y su mitigación se gestionan en la consola de OCI. Verifícalas antes del despliegue.
- **Ampliación del volumen (200 GB a más de 500 GB):** se hace a mano la semana anterior, con los pasos de consola, partición y sistema de archivos. `deploy/host/` solo contiene un marcador; no hay procedimiento en el repositorio.
- **Versión estable y rollback:** el pinning de `BODA_TAG` y la vuelta atrás también los gestiona el operador.

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
