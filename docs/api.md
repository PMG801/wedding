# Contrato HTTP

## Alcance de la fuente

La definición técnica aportada describe flujos y algunas propiedades de las rutas, pero no incluye el bloque completo de contrato HTTP que anuncia para un documento posterior. No especifica para cada endpoint todos los esquemas JSON, cabeceras, parámetros, códigos de error ni ejemplos. Lo siguiente recoge lo que sí está definido; no completa los campos ausentes por inferencia.

## Endpoints

### `GET /e/{token}` — acceso de invitado por QR

El QR apunta a `/e/{token}`. La aplicación comprueba el token, crea una cookie de invitado firmada (`HttpOnly`, `Secure`, `SameSite=Lax`, 30 días) y redirige a `/`, para que el token no se quede en la barra de direcciones ni salga en capturas de pantalla.

### `POST /api/admin/login` — inicio de sesión de administrador

Cuerpo JSON de como máximo 4 KiB, sin campos adicionales:

```json
{"password":"<contraseña>"}
```

La contraseña se compara con `BODA_ADMIN_PASSWORD_HASH` (bcrypt o argon2id) en tiempo constante.

Retraso progresivo: tras cada fallo, el siguiente intento espera antes de verificarse, 1 s tras el primer fallo, 2 s tras el segundo, 4 s, y así hasta 30 s como máximo. Un acierto restablece el contador. El contador es global (no por IP) y vive solo en memoria, así que se reinicia con el proceso.

| Estado | Significado y respuesta |
| --- | --- |
| `204 No Content` | Contraseña correcta; cuerpo vacío y cookie `admin_session`. |
| `400 Bad Request` | JSON no válido, campo desconocido o tipo incorrecto (`{"error":"invalid_request"}`). |
| `401 Unauthorized` | Contraseña incorrecta (`{"error":"invalid_credentials"}`). |
| `413 Request Entity Too Large` | Cuerpo mayor de 4 KiB (`{"error":"request_too_large"}`). |

Cookie `admin_session`: `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`, `Max-Age=43200` (12 horas). Va firmada con la clave de sesión de la aplicación, pero con un dominio de firma distinto al de la cookie de invitado: ninguna de las dos sirve en la ruta de la otra. Todas las respuestas de `/api/admin/*` llevan `Cache-Control: no-store`.

### `POST /api/admin/logout` — cierre de sesión

No exige sesión. Responde `204 No Content` y envía una cookie `admin_session` vacía con `Max-Age=-1`, que hace que el navegador la elimine. La cookie es firmada y sin estado en el servidor: una copia robada sigue siendo válida hasta caducar (12 horas). Cerrar sesión solo borra la cookie del navegador que lo pide.

### `GET /api/admin/status` — estado de subidas y disco

Exige la sesión de administrador. Responde `200 OK` con `Content-Type: application/json; charset=utf-8`:

```json
{
  "uploadsEnabled": true,
  "diskPaused": false,
  "freeBytes": 10000000000,
  "reservedBytes": 1000000000,
  "minFreeBytes": 16106127360
}
```

- `uploadsEnabled`: el interruptor manual (`uploads_enabled` en SQLite; sin fila, `true`).
- `diskPaused`: `true` cuando `freeBytes - reservedBytes < minFreeBytes` (o cuando `reservedBytes` supera `freeBytes`). Es informativo: el espacio real lo aplica cada subida con `507 insufficient_storage`.
- `freeBytes`, `reservedBytes` y `minFreeBytes`: bytes libres del volumen, bytes reservados por subidas en curso y umbral configurado en `BODA_DISK_MIN_FREE_BYTES`.

| Estado | Significado y respuesta |
| --- | --- |
| `200 OK` | Estado actual. |
| `401 Unauthorized` | Falta una sesión de administrador válida, o la cookie es de invitado (`{"error":"admin_session_required"}`). |
| `503 Service Unavailable` | No se puede medir el disco (`{"error":"status_unavailable"}`). |

### `POST /api/admin/settings` — interruptor de subidas

Exige la sesión de administrador. Cuerpo JSON con el campo obligatorio `uploadsEnabled` (booleano) y sin campos adicionales:

```json
{"uploadsEnabled":false}
```

El cambio se guarda en SQLite (clave `uploads_enabled`, ver `docs/data-model.md`) antes de aplicarse en memoria, sobrevive a reinicios y surte efecto en la siguiente petición de subida. Con `false`, `PUT /api/media/{id}` y `POST /api/media/{id}/derived` responden `503 uploads_paused`.

| Estado | Significado y respuesta |
| --- | --- |
| `200 OK` | Interruptor guardado; cuerpo con el mismo JSON que `GET /api/admin/status`. |
| `400 Bad Request` | Cuerpo no válido, campo ausente, tipo incorrecto o campo desconocido (`{"error":"invalid_request"}`). |
| `401 Unauthorized` | Sin sesión de administrador (`{"error":"admin_session_required"}`). |
| `500 Internal Server Error` | No se ha podido guardar el ajuste (`{"error":"settings_failed"}`); el interruptor no cambia. |
| `503 Service Unavailable` | El ajuste se ha guardado, pero no se ha podido medir el disco para la respuesta (`{"error":"status_unavailable"}`). |

Si el servidor arranca sin configuración de administración, las rutas `status` y `settings` responden `503 {"error":"admin_unavailable"}` después de comprobar la sesión.

### `POST /api/media/{id}/derived?variant=t|d` — versión derivada JPEG

La ruta exige una cookie de sesión de invitado válida (`guest_session`) y un UUIDv4 canónico en minúsculas. Cada petición sube una variante JPEG en el cuerpo sin multipart: `variant=t` para la miniatura (unos 400 px) o `variant=d` para el detalle (unos 2.048 px). El cliente envía `Content-Type: image/jpeg` y un `Content-Length` positivo. El servidor valida la firma JPEG y aplica los mismos límites de tamaño (`BODA_MAX_PHOTO_BYTES`), concurrencia (`BODA_MAX_CONCURRENT_UPLOADS`), inactividad y espacio mínimo libre que a la subida del original.

Las variantes pueden subirse antes que el original. En ese caso quedan en almacenamiento privado y no se sirven ni aparecen en la galería; se publican al confirmar un original visible con el mismo ID. Una variante ya aceptada no se vuelve a escribir en un reintento. Si falta una variante, el cliente puede continuar con el fallback de original sin procesamiento en el servidor.

| Estado | Significado y respuesta |
| --- | --- |
| `201 Created` | Derivado nuevo aceptado; cuerpo vacío. Si aún no existe el original, queda privado hasta su confirmación. |
| `200 OK` | Reintento de un derivado ya aceptado; cuerpo vacío y sin reescritura. |
| `400 Bad Request` | UUID no canónico (`{"error":"invalid_media_id"}`), variante no admitida (`{"error":"invalid_derivative_variant"}`), cuerpo vacío (`{"error":"empty_derivative"}`) o tamaño real distinto del declarado (`{"error":"derivative_body_size_mismatch"}`). |
| `401 Unauthorized` | Falta una sesión de invitado válida; cuerpo de texto `guest session required\n`. |
| `404 Not Found` | El medio existente está oculto (`{"error":"media_not_found"}`). |
| `405 Method Not Allowed` | Método distinto de `POST`; el cuerpo es el texto estándar de `net/http`. |
| `408 Request Timeout` | El cuerpo no recibe datos durante el plazo de inactividad configurado (`{"error":"upload_idle_timeout"}`). |
| `411 Length Required` | Falta un tamaño declarado (`{"error":"content_length_required"}`). |
| `413 Request Entity Too Large` | El tamaño declarado supera el máximo configurado (`{"error":"derivative_too_large"}`). |
| `415 Unsupported Media Type` | El tipo declarado no es `image/jpeg` o la firma no es JPEG (`{"error":"unsupported_derivative_format"}`). |
| `507 Insufficient Storage` | El espacio libre no alcanza el umbral mínimo configurado (`{"error":"insufficient_storage"}`). |
| `503 Service Unavailable` | Capacidad compartida de subidas completa: respuesta inmediata, `Retry-After: 1` y `{"error":"upload_capacity_reached"}`; si el servicio de derivados no está disponible, `{"error":"derivatives_unavailable"}`. |
| `503 Service Unavailable` (derivados) | Subidas en pausa con el interruptor de administración: `{"error":"uploads_paused"}`, sin leer el cuerpo ni reservar capacidad. |
| `500 Internal Server Error` | Error interno al guardar el derivado (`{"error":"derivative_upload_failed"}`). |

### `PUT /api/media/{id}` — original de foto o vídeo en streaming

La ruta exige una cookie de sesión de invitado válida (`guest_session`). Recibe los bytes originales directamente en el cuerpo; no acepta carga multipart. `{id}` debe ser un UUIDv4 canónico en minúsculas y se exige `Content-Length` positivo. El límite depende del tipo detectado por firma: `BODA_MAX_PHOTO_BYTES` (50 MB por defecto) para JPEG, PNG, HEIC y HEIF, y `BODA_MAX_VIDEO_BYTES` (1,5 GB por defecto) para vídeo MP4/M4V y QuickTime MOV (marca principal `isom`, `iso2`, `mp41`, `mp42`, `avc1`, `M4V ` o `qt  `). No se transcodifica nada en el servidor. Los derivados (miniatura y detalle, también la portada de un vídeo) siguen usando el límite de foto.

Caddy reenvía el cuerpo sin buffering. El plazo de lectura es de inactividad (90 segundos por defecto), no un plazo total de subida. La aplicación escribe en `tmp/{id}.part`, valida firma y tamaño, sincroniza el archivo, lo renombra y confirma en SQLite. Un identificador ya confirmado devuelve el mismo éxito idempotente sin volver a escribir.

| Estado | Significado y respuesta |
| --- | --- |
| `201 Created` | Foto nueva confirmada; cuerpo vacío. |
| `200 OK` | Reintento de una foto ya confirmada; cuerpo vacío y sin reescritura. |
| `400 Bad Request` | UUID no canónico (`{"error":"invalid_media_id"}`), cuerpo vacío (`{"error":"empty_photo"}`) o tamaño real distinto del declarado (`{"error":"photo_body_size_mismatch"}`). |
| `401 Unauthorized` | Falta una sesión de invitado válida; cuerpo de texto `guest session required\n`. |
| `405 Method Not Allowed` | Método distinto de `PUT`; el cuerpo es el texto estándar de `net/http`. |
| `411 Length Required` | Falta un tamaño declarado (`{"error":"content_length_required"}`). |
| `413 Request Entity Too Large` | El tamaño declarado supera el máximo de foto (`{"error":"photo_too_large"}`), el de vídeo (`{"error":"video_too_large"}`) o, antes de leer el cuerpo y cuando el límite de vídeo es mayor que el de foto, el mayor de los dos (`{"error":"media_too_large"}`). |
| `408 Request Timeout` | El cuerpo no recibe datos durante el plazo de inactividad configurado (`{"error":"upload_idle_timeout"}`). |
| `415 Unsupported Media Type` | La firma no es JPEG, PNG, HEIC, HEIF, MP4 ni MOV (`{"error":"unsupported_photo_format"}`). |
| `507 Insufficient Storage` | El espacio libre no alcanza el umbral mínimo configurado (`{"error":"insufficient_storage"}`). |
| `500 Internal Server Error` | Error interno al persistir la foto (`{"error":"upload_failed"}`). |
| `503 Service Unavailable` | Capacidad de subidas completa: respuesta inmediata, `Retry-After: 1` y `{"error":"upload_capacity_reached"}`. Si el servicio de subidas no está disponible, responde `{"error":"uploads_unavailable"}`. |
| `503 Service Unavailable` | Subidas en pausa con el interruptor de administración (`uploads_enabled` en `false`): respuesta inmediata, sin leer el cuerpo, sin `Retry-After` y `{"error":"uploads_paused"}`. Las lecturas (galería, sesión, medios) siguen funcionando. |

Los errores JSON usan `Content-Type: application/json; charset=utf-8` y el formato `{"error":"<código>"}` seguido de salto de línea. La capacidad máxima simultánea se configura con `BODA_MAX_CONCURRENT_UPLOADS` (30 por defecto); una petición que encuentre todos los espacios ocupados se rechaza sin esperar ni leer su cuerpo.

### `GET /api/media?cursor=<opaco>&limit=50` — galería

La ruta exige una cookie de sesión de invitado válida (`guest_session`). Devuelve solo medios confirmados y visibles, del más reciente al más antiguo. `limit` es opcional (entre 1 y 100; 50 por defecto). `cursor` es opcional y opaco: el cliente debe reenviar sin modificar el valor `next_cursor` de la página anterior. La paginación usa el par `(confirmed_at, id)`, por lo que las subidas nuevas no desplazan las páginas siguientes.

```json
{
  "items": [
    {
      "id": "123e4567-e89b-42d3-a456-426614174000",
      "mime": "image/heic",
      "confirmed_at": 1790000000000,
      "original_url": "/m/o/20260926T183012.000000000Z_123e4567-e89b-42d3-a456-426614174000.heic",
      "thumb_url": "/m/d/123e4567-e89b-42d3-a456-426614174000_t.jpg",
      "detail_url": null
    }
  ],
  "next_cursor": null
}
```

`confirmed_at` son milisegundos Unix UTC. `thumb_url` y `detail_url` valen `null` mientras la variante no esté publicada en disco; el cliente muestra entonces un marcador y nunca intenta pintar un original HEIC en un `<img>`. `next_cursor` vale `null` en la última página.

| Estado | Significado y respuesta |
| --- | --- |
| `200 OK` | Página JSON con `Content-Type: application/json; charset=utf-8` y `Cache-Control: no-store`. Una galería vacía devuelve `{"items":[],"next_cursor":null}`. |
| `400 Bad Request` | `limit` fuera de rango o no numérico (`{"error":"invalid_limit"}`), o cursor no emitido por el servidor (`{"error":"invalid_cursor"}`). |
| `401 Unauthorized` | Falta una sesión de invitado válida; cuerpo de texto `guest session required\n`. |
| `500 Internal Server Error` | Error interno al leer la galería (`{"error":"gallery_failed"}`). |
| `503 Service Unavailable` | El servicio de galería no está disponible (`{"error":"gallery_unavailable"}`). |

### `GET /api/session` — comprobación de sesión de invitado

Responde `204 No Content` con `Cache-Control: no-store` si la cookie `guest_session` es válida, y `401 Unauthorized` (texto `guest session required\n`) si falta, está alterada o ha caducado. Caddy la usa con `forward_auth` para proteger los medios, y el frontend puede usarla para saber si el invitado ya entró por el QR.

### `GET /m/o/{archivo}` y `GET /m/d/{archivo}` — medios publicados

Los sirve Caddy, no la aplicación (ADR-0006 y ADR-0011). Exigen la sesión de invitado: sin ella responden `401`. `/m/o/` sirve los originales visibles y `/m/d/` los derivados JPEG publicados. Las URL no se construyen en el cliente; se toman de `original_url`, `thumb_url` y `detail_url` de la galería.

- `200 OK` con `Cache-Control: private, max-age=31536000, immutable`, `X-Content-Type-Options: nosniff`, sin compresión, y `Content-Type: image/heic` o `image/heif` para esas extensiones.
- `206 Partial Content` para peticiones `Range`.
- `?dl=1` añade `Content-Disposition: attachment` para forzar la descarga.
- `404 Not Found` para archivos ocultos, aún no publicados o fuera de `originals/` y `derived/`; esta respuesta no se marca como cacheable.

### `PATCH /api/admin/media/{id}` — moderación

Las rutas `/api/admin/*` exigen la sesión de administrador. Ocultar un medio mueve el archivo a `hidden/` (no servido) y actualiza su estado en SQLite; su URL da 404 al instante. La aplicación permite al administrador ver los elementos ocultos. El cuerpo exacto para ocultar/mostrar y los códigos de respuesta no se especifican en la fuente.

### `POST /api/admin/settings` — ajustes

Las rutas `/api/admin/*` exigen la sesión de administrador. El interruptor manual de subidas se guarda en SQLite y puede cambiarse en marcha. El material entregado no define el esquema del cuerpo ni la respuesta de este endpoint.

## Autenticación y límites comunes

- El acceso de invitado se obtiene mediante el token QR convertido en cookie firmada; el token no permanece en la URL tras la redirección.
- Las rutas administrativas exigen la sesión independiente del administrador.
- Los límites de tamaño, concurrencia, inactividad y espacio se aplican en el servidor; el cliente puede replicarlos solo para mostrar mejores mensajes.
- La ruta de originales admite JPEG, HEIC, HEIF y PNG, además de vídeo MP4 y MOV, por firma.

## Información pendiente para completar el contrato

El esquema de moderación (`PATCH /api/admin/media/{id}`) sigue pendiente de definición. Los endpoints de sesión de administrador (`login`, `logout`), `status` y `settings` están documentados arriba.

### `GET /api/health` — estado del servicio

Devuelve `200 OK` con `Content-Type: application/json` y el cuerpo `{"status":"ok"}`. Los demás métodos no están permitidos y reciben `405 Method Not Allowed`.
