# ADR-0010 · Entrega y copia: fuera de la aplicación

- **Decisión:**
  - El operador hace `rsync` de `originals/`, y de `hidden/` si se pide, a un disco externo.
  - Opcionalmente, un `rsync` incremental la noche del evento, que vuelve más corto el final.
  - La copia de SQLite no forma parte de la entrega a los novios.
- **Motivo:** tu decisión explícita. Los archivos no cambian y los temporales están aparte, así que el `rsync` es seguro **con la aplicación en marcha**.
- **Consecuencias:** hasta ese `rsync`, la única copia de los archivos es el volumen del servidor. Se acepta.
