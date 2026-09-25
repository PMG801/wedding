# Modelo de datos y estados

## Persistencia

SQLite embebida, con el driver `modernc.org/sqlite` (Go puro), en `data/boda.db`. Se abre en modo WAL, con `busy_timeout` de 5 s, `synchronous=NORMAL` y claves foráneas activas. Las migraciones se guardan como archivos SQL embebidos numerados y se controlan con `PRAGMA user_version`. La copia nocturna se hace con `VACUUM INTO` a `data/backup/`. El índice se puede reconstruir desde el disco con `boda reindex`.

La base de datos guarda metadatos, estados, cursores de la galería, cola de tareas, configuración modificable en marcha y credenciales. No se mantiene ninguna transacción abierta durante una subida y no se guarda el progreso de la subida. Nunca se copia el archivo de la base de datos en caliente.

## Esquema SQL

El texto técnico entregado no contiene el bloque `0001_init.sql` ni el DDL de las tablas `settings` y `media`; solo los anuncia como parte de un bloque posterior no incluido. Por tanto, no se inventan columnas, tipos, restricciones ni índices. Para completar esta sección hace falta aportar el SQL íntegro, incluidos los índices parciales de galería y estado.

## Máquina de estados desacoplada

- **El disco define la existencia:** la escritura temporal se confirma mediante `fsync` y un renombrado atómico dentro del mismo sistema de archivos. Los archivos publicados no se modifican.
- **SQLite define la visibilidad:** la base de datos contiene el estado/metadato que determina si el archivo aparece en la galería.
- El orden de confirmación es: escritura forzada, renombrado y confirmación en la base de datos.
- **No se guardan estados de progreso de subida en la base de datos.** No debe quedar ningún registro `uploading`.
- El renombrado entre ubicaciones requiere un único sistema de archivos; separar `tmp/`, `originals/` y `hidden/` en montajes distintos causa `EXDEV`.
- Al ocultar, el archivo se mueve a `hidden/` y se actualiza su estado en SQLite. El directorio `hidden/` no se sirve.
- El índice puede reconstruirse desde el disco con `boda reindex`. Los archivos de `originals/` sin registro van a `quarantine/`, nunca se borran.

La definición resume la máquina como: **el disco decide si un archivo existe; SQLite decide si es visible**. Si se corrompe la base de datos, se pierden estados y visibilidad, pero ningún archivo.
