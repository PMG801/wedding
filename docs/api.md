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

### `PUT /api/media/{id}` — original en streaming

- El identificador lo genera el cliente. El original se sube con una petición `PUT`, con el archivo como cuerpo y su tamaño declarado.
- Caddy reenvía sin buffering; no se fija un plazo total. El plazo de inactividad es 90 segundos.
- La aplicación escribe en `tmp/{id}.part`, comprueba la firma del archivo y el tamaño, fuerza la escritura a disco (`fsync`), renombra, confirma en SQLite y responde 201.
- Se comprueba el espacio disponible. Por debajo de `BODA_DISK_MIN_FREE_BYTES` (15 GB por defecto) se desactivan automáticamente las subidas.
- El máximo predeterminado de concurrencia es 30. Por encima se responde 503 con `Retry-After`.
- Se aceptan JPEG, HEIC, HEIF, PNG, MP4 y MOV, comprobando la firma del archivo; esto excluye SVG y HTML.
- Si se reintenta un identificador ya confirmado, responde 200 sin volver a escribir. Si se reintenta uno que está subiéndose, el nuevo intento sustituye al antiguo.
- El cliente declara el tamaño. Los máximos configurables de foto y vídeo son 50 MB y 1,5 GB, respectivamente.

La definición técnica no incluye la lista exhaustiva de códigos de error ni la especificación formal de cabeceras para este endpoint.

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
- La lista de tipos permitidos es JPEG, HEIC, HEIF, PNG, MP4 y MOV, validada mediante firma del archivo.

## Información pendiente para completar el contrato

Hace falta el bloque de contrato HTTP anunciado en la definición final, con formatos exactos de petición/respuesta, cabeceras, códigos de estado y error, paginación por cursor y semántica completa del endpoint de derivados.

### `GET /api/health` — estado del servicio

Devuelve `200 OK` con `Content-Type: application/json` y el cuerpo `{"status":"ok"}`. Los demás métodos no están permitidos y reciben `405 Method Not Allowed`.
