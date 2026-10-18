# Modelo de datos y estados

## Persistencia

SQLite embebida, con el driver `modernc.org/sqlite` (Go puro), en `data/boda.db`. Se abre en modo WAL, con `busy_timeout=5000` ms, `synchronous=FULL`, `journal_size_limit` de 64 MiB y claves foráneas activas. `synchronous=FULL` espera al disco en cada confirmación: una caída de energía no pierde subidas ya confirmadas, a cambio de algo más de latencia en escritura. Los pragmas que dependen de cada conexión se configuran en el DSN, de modo que se aplican también a las conexiones que `database/sql` cree después de abrir la base.

Las migraciones son archivos SQL numerados e incrustados en el binario. `PRAGMA user_version` registra la última migración aplicada. Cada migración y su actualización de versión se ejecutan en una transacción: un fallo revierte ambos cambios. Una migración ya aplicada no vuelve a ejecutarse; si la base tiene una versión superior a la que admite el binario, la apertura falla sin rebajarla.

## Esquema SQL inicial

La migración `0001_initial.sql` crea únicamente `media` y `settings`; no inserta valores iniciales ni crea tablas de cuentas, sesiones, credenciales, cola de tareas o progreso de subida.

| Tabla y columna | Tipo / restricción | Significado |
|---|---|---|
| `media.id` | `TEXT PRIMARY KEY NOT NULL` | Identificador recibido para la operación idempotente. |
| `media.storage_name` | `TEXT NOT NULL UNIQUE` | Nombre de almacenamiento asignado en el servidor. |
| `media.media_type` | `TEXT NOT NULL` | Tipo MIME del medio. |
| `media.size_bytes` | `INTEGER NOT NULL`, mayor que cero | Tamaño del archivo en bytes. |
| `media.confirmed_at` | `INTEGER NOT NULL` | Instante de confirmación como milisegundos Unix UTC. |
| `media.visibility` | `TEXT NOT NULL DEFAULT 'visible'`, solo `visible` o `hidden` | Estado de visibilidad en la galería. |
| `settings.key` | `TEXT PRIMARY KEY NOT NULL` | Clave de configuración. |
| `settings.value` | `TEXT NOT NULL` | Valor asociado a la clave. |
| `settings.updated_at` | `INTEGER NOT NULL` | Marca de actualización entera. |

Los tamaños se validan además como enteros positivos. Hay dos índices parciales para la paginación cronológica, ambos ordenados por `(confirmed_at DESC, id DESC)`: uno para filas `visible` y otro para filas `hidden`. `storage_name` es único. La restricción de visibilidad no incluye estados de subida.

Esta migración establece solo la forma de almacenamiento y no define nombres de claves ni políticas de producto. La capa de aplicación valida las claves de `settings` contra una lista permitida (ver "Claves de `settings`"). Los índices se llaman `media_visible_cursor` y `media_hidden_cursor`; cada uno filtra por su estado correspondiente y ordena por `(confirmed_at DESC, id DESC)`.

## Migración `0002_media_likes.sql`

Añade los «me gusta» (ADR-0012):

| Tabla y columna | Tipo / restricción | Significado |
|---|---|---|
| `media.like_count` | `INTEGER NOT NULL DEFAULT 0`, entero mayor o igual que cero | Número de filas de `media_likes` del medio. |
| `media_likes.media_id` | `TEXT NOT NULL`, referencia a `media.id` | Medio marcado. |
| `media_likes.guest_id` | `TEXT NOT NULL`, 32 caracteres hexadecimales | Identidad anónima de la cookie de invitado. |
| `media_likes.created_at` | `INTEGER NOT NULL` | Instante del «me gusta» en milisegundos Unix UTC. |

La clave primaria `(media_id, guest_id)` impide dos «me gusta» de la misma identidad al mismo medio. Insertar o borrar la fila y ajustar `media.like_count` ocurren en la misma transacción, así que el recuento no se desajusta ni se pierde al reiniciar. El índice parcial `media_visible_popularity` ordena por `(like_count DESC, confirmed_at DESC, id DESC)` las filas `visible` con `like_count > 0`, para el ranking de las 50 más gustadas. Ocultar un medio no borra sus «me gusta».

## Claves de `settings`

La tabla `settings` guarda solo ajustes operativos, nunca secretos. La capa de aplicación acepta únicamente las claves de una lista permitida; cualquier otra clave se rechaza sin escribir nada.

| Clave | Valores | Ausencia de fila | Significado |
|---|---|---|---|
| `uploads_enabled` | `true` o `false` (texto exacto, en minúsculas) | Equivale a `true` | Interruptor manual de subidas (ADR-0008). Con `false`, las subidas de originales y derivados responden `503 uploads_paused`. |

Reglas de `uploads_enabled`:

- Se escribe desde `POST /api/admin/settings` con `INSERT ... ON CONFLICT(key) DO UPDATE`, con `updated_at` en milisegundos Unix UTC.
- Al arrancar se lee una vez. Un valor distinto de `true` o `false` impide arrancar el servicio.
- En ejecución, las subidas consultan una caché en memoria y no SQLite. Cada cambio se escribe primero en la base de datos y después en la caché; si la escritura falla, la caché no cambia.
- La pausa automática por espacio libre (`diskPaused` en el estado) no se guarda aquí: se calcula en cada consulta a partir del disco.

## Máquina de estados desacoplada

- **El disco define la existencia:** la escritura temporal se confirma mediante `fsync` y un renombrado atómico dentro del mismo sistema de archivos. Los archivos publicados no se modifican.
- **SQLite define la visibilidad:** la base de datos contiene el estado/metadato que determina si el archivo aparece en la galería.
- El orden de confirmación es: escritura forzada, renombrado y confirmación en la base de datos.
- **No se guardan estados de progreso de subida en la base de datos.** No debe quedar ningún registro `uploading`.
- El renombrado entre ubicaciones requiere un único sistema de archivos; separar `tmp/`, `originals/` y `hidden/` en montajes distintos causa `EXDEV`.
- Al ocultar, el archivo se mueve a `hidden/` y se actualiza su estado en SQLite. El directorio `hidden/` no se sirve. Sus derivados vuelven a `tmp/` como variantes privadas (`*.staged`), así `hidden/` solo contiene originales para la entrega. Nunca se borra un medio: ocultar es reversible.
- El índice puede reconstruirse desde el disco con `boda reindex`. Los archivos de `originals/` sin registro van a `quarantine/`, nunca se borran.

La definición resume la máquina como: **el disco decide si un archivo existe; SQLite decide si es visible**. Si se corrompe la base de datos, se pierden estados y visibilidad, pero ningún archivo.
