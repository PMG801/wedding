# Modelo de datos y estados

## Persistencia

SQLite embebida, con el driver `modernc.org/sqlite` (Go puro), en `data/boda.db`. Se abre en modo WAL, con `busy_timeout=5000` ms, `synchronous=NORMAL` y claves foráneas activas. Los pragmas que dependen de cada conexión se configuran en el DSN, de modo que se aplican también a las conexiones que `database/sql` cree después de abrir la base.

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

Esta migración establece solo la forma de almacenamiento y no define nombres de claves ni políticas de producto. La capa de aplicación debe validar las claves de `settings` contra una lista permitida al implementar sus operaciones; no se deben guardar secretos allí. Los índices se llaman `media_visible_cursor` y `media_hidden_cursor`; cada uno filtra por su estado correspondiente y ordena por `(confirmed_at DESC, id DESC)`.

## Máquina de estados desacoplada

- **El disco define la existencia:** la escritura temporal se confirma mediante `fsync` y un renombrado atómico dentro del mismo sistema de archivos. Los archivos publicados no se modifican.
- **SQLite define la visibilidad:** la base de datos contiene el estado/metadato que determina si el archivo aparece en la galería.
- El orden de confirmación es: escritura forzada, renombrado y confirmación en la base de datos.
- **No se guardan estados de progreso de subida en la base de datos.** No debe quedar ningún registro `uploading`.
- El renombrado entre ubicaciones requiere un único sistema de archivos; separar `tmp/`, `originals/` y `hidden/` en montajes distintos causa `EXDEV`.
- Al ocultar, el archivo se mueve a `hidden/` y se actualiza su estado en SQLite. El directorio `hidden/` no se sirve.
- El índice puede reconstruirse desde el disco con `boda reindex`. Los archivos de `originals/` sin registro van a `quarantine/`, nunca se borran.

La definición resume la máquina como: **el disco decide si un archivo existe; SQLite decide si es visible**. Si se corrompe la base de datos, se pierden estados y visibilidad, pero ningún archivo.
