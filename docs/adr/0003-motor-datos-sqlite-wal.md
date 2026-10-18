# ADR-0003 · Motor de datos: SQLite en modo WAL

- **Contexto:** metadatos de unos pocos miles de archivos y decenas de escrituras por minuto en el peor caso.
- **Decisión:**
  - SQLite embebida, con el driver `modernc.org/sqlite` (Go puro), en `data/boda.db`.
  - Al abrirla: modo WAL, `busy_timeout` de 5 s, `synchronous=FULL`, `journal_size_limit` de 64 MiB y claves foráneas activas. `FULL` sincroniza el WAL en cada confirmación: con una escritura por subida el coste es despreciable, y una subida confirmada sobrevive a un corte de corriente, no solo a la caída del proceso.
  - Migraciones en SQL embebido, con `user_version`.
  - Copia periódica con `VACUUM INTO` a `data/backup/` (`BODA_BACKUP_INTERVAL`, 24 h por defecto; se conservan las `BODA_BACKUP_KEEP` más recientes, 7 por defecto). Se escribe con nombre temporal y se renombra, así que nunca queda una copia truncada.
  - El índice se puede reconstruir desde el disco con `boda reindex`.
- **Motivo determinante:** no hay ningún proceso que operar y la copia es un único archivo. El modelo de un solo escritor sobra para esta carga.
- **Descartado:**
  - PostgreSQL: otro servicio sin ganancia.
  - El driver con código C: rompe la compilación cruzada.
  - Un ORM: con tan pocas tablas, no aporta nada.
- **Consecuencias, con reglas obligatorias:**
  - Ninguna transacción abierta mientras dura una subida.
  - No se guarda el progreso de la subida.
  - Nunca se copia el archivo de la base de datos en caliente.
  - Si se corrompe la base de datos, se pierden estados y visibilidad, pero **ningún archivo**.
