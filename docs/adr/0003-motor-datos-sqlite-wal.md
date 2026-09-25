# ADR-0003 · Motor de datos: SQLite en modo WAL

- **Contexto:** metadatos de unos pocos miles de archivos y decenas de escrituras por minuto en el peor caso.
- **Decisión:**
  - SQLite embebida, con el driver `modernc.org/sqlite` (Go puro), en `data/boda.db`.
  - Al abrirla: modo WAL, `busy_timeout` de 5 s, `synchronous=NORMAL` y claves foráneas activas.
  - Migraciones en SQL embebido, con `user_version`.
  - Copia cada noche con `VACUUM INTO` a `data/backup/`.
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
