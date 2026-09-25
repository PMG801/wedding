# ADR-0009 · Limpieza y conciliación

- **Decisión:** la tabla de la limpieza de huérfanos ya aprobada, ejecutada al arrancar y cada 10 minutos.
  - Lo que está a medio subir se borra.
  - Lo que está en `originals/` sin registro va a `quarantine/`, **nunca se borra**.
- **Consecuencias:** `quarantine/` se revisa a mano antes del `rsync` final.
