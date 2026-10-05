# Contrato HTTP

## Alcance de la fuente

La definición técnica aportada describe flujos y algunas propiedades de las rutas, pero no incluye el bloque completo de contrato HTTP que anuncia para un documento posterior. No especifica para cada endpoint todos los esquemas JSON, cabeceras, parámetros, códigos de error ni ejemplos. Lo siguiente recoge lo que sí está definido; no completa los campos ausentes por inferencia.

## Endpoints

### `GET /e/{token}` — acceso de invitado por QR

El QR apunta a `/e/{token}`. La aplicación comprueba el token, crea una cookie de invitado firmada (`HttpOnly`, `Secure`, `SameSite=Lax`, 30 días) y redirige a `/`, para que el token no se quede en la barra de direcciones ni salga en capturas de pantalla.

### `POST /api/admin/login` — inicio de sesión de administrador

El administrador entra con una contraseña con hash y una cookie de sesión aparte (`SameSite=Strict`). El login aplica un retraso progresivo tras fallos. La definición no proporciona el esquema del cuerpo ni la forma exacta de la cookie, sus atributos completos o los códigos de respuesta.

### `POST /api/media/{id}/derived` — versiones derivadas

El cliente genera dos JPEG para las fotos: `_t` (unos 400 px) y `_d` (unos 2.048 px); en vídeo, genera una portada en el dispositivo. Las versiones pequeñas se suben primero, en peticiones cortas. La descripción solicitada del endpoint contempla la subida de `_t` y `_d`, pero la fuente no define la codificación de la petición, la convención exacta del cuerpo ni sus respuestas.

### `PUT /api/media/{id}` — original de foto en streaming

La ruta exige una cookie de sesión de invitado válida (`guest_session`). Recibe los bytes originales directamente en el cuerpo; no acepta carga multipart. `{id}` debe ser un UUIDv4 canónico en minúsculas y se exige `Content-Length` positivo. El límite es `BODA_MAX_PHOTO_BYTES` (50 MB por defecto). Esta ruta admite JPEG, PNG, HEIC y HEIF mediante comprobación de firma; no admite vídeo.

Caddy reenvía el cuerpo sin buffering. El plazo de lectura es de inactividad (90 segundos por defecto), no un plazo total de subida. La aplicación escribe en `tmp/{id}.part`, valida firma y tamaño, sincroniza el archivo, lo renombra y confirma en SQLite. Un identificador ya confirmado devuelve el mismo éxito idempotente sin volver a escribir.

| Estado | Significado y respuesta |
| --- | --- |
| `201 Created` | Foto nueva confirmada; cuerpo vacío. |
| `200 OK` | Reintento de una foto ya confirmada; cuerpo vacío y sin reescritura. |
| `400 Bad Request` | UUID no canónico (`{"error":"invalid_media_id"}`), cuerpo vacío (`{"error":"empty_photo"}`) o tamaño real distinto del declarado (`{"error":"photo_body_size_mismatch"}`). |
| `401 Unauthorized` | Falta una sesión de invitado válida; cuerpo de texto `guest session required\n`. |
| `405 Method Not Allowed` | Método distinto de `PUT`; el cuerpo es el texto estándar de `net/http`. |
| `411 Length Required` | Falta un tamaño declarado (`{"error":"content_length_required"}`). |
| `413 Request Entity Too Large` | El tamaño declarado supera el máximo de foto (`{"error":"photo_too_large"}`). |
| `408 Request Timeout` | El cuerpo no recibe datos durante el plazo de inactividad configurado (`{"error":"upload_idle_timeout"}`). |
| `415 Unsupported Media Type` | La firma no es JPEG, PNG, HEIC ni HEIF (`{"error":"unsupported_photo_format"}`). |
| `507 Insufficient Storage` | El espacio libre no alcanza el umbral mínimo configurado (`{"error":"insufficient_storage"}`). |
| `500 Internal Server Error` | Error interno al persistir la foto (`{"error":"upload_failed"}`). |
| `503 Service Unavailable` | Capacidad de subidas completa: respuesta inmediata, `Retry-After: 1` y `{"error":"upload_capacity_reached"}`. Si el servicio de subidas no está disponible, responde `{"error":"uploads_unavailable"}`. |

Los errores JSON usan `Content-Type: application/json; charset=utf-8` y el formato `{"error":"<código>"}` seguido de salto de línea. La capacidad máxima simultánea se configura con `BODA_MAX_CONCURRENT_UPLOADS` (30 por defecto); una petición que encuentre todos los espacios ocupados se rechaza sin esperar ni leer su cuerpo.

### `GET /api/media` — galería

La galería usa cursores cronológicos. Los metadatos y cursores de galería se guardan en SQLite. La forma del cursor, parámetros de paginación y esquema de respuesta no están incluidos en el texto fuente.

### `PATCH /api/admin/media/{id}` — moderación

Las rutas `/api/admin/*` exigen la sesión de administrador. Ocultar un medio mueve el archivo a `hidden/` (no servido) y actualiza su estado en SQLite; su URL da 404 al instante. La aplicación permite al administrador ver los elementos ocultos. El cuerpo exacto para ocultar/mostrar y los códigos de respuesta no se especifican en la fuente.

### `POST /api/admin/settings` — ajustes

Las rutas `/api/admin/*` exigen la sesión de administrador. El interruptor manual de subidas se guarda en SQLite y puede cambiarse en marcha. El material entregado no define el esquema del cuerpo ni la respuesta de este endpoint.

## Autenticación y límites comunes

- El acceso de invitado se obtiene mediante el token QR convertido en cookie firmada; el token no permanece en la URL tras la redirección.
- Las rutas administrativas exigen la sesión independiente del administrador.
- Los límites de tamaño, concurrencia, inactividad y espacio se aplican en el servidor; el cliente puede replicarlos solo para mostrar mejores mensajes.
- La ruta de originales de foto documentada aquí admite JPEG, HEIC, HEIF y PNG por firma; la subida de vídeo no forma parte de este endpoint.

## Información pendiente para completar el contrato

Hace falta el bloque de contrato HTTP anunciado en la definición final, con formatos exactos de petición/respuesta, cabeceras, códigos de estado y error, paginación por cursor y semántica completa del endpoint de derivados.

### `GET /api/health` — estado del servicio

Devuelve `200 OK` con `Content-Type: application/json` y el cuerpo `{"status":"ok"}`. Los demás métodos no están permitidos y reciben `405 Method Not Allowed`.
