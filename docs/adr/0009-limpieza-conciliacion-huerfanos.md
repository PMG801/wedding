# ADR-0009 · Limpieza y conciliación

- **Decisión:** la tabla de la limpieza de huérfanos ya aprobada, ejecutada al arrancar y cada 10 minutos.
  - Lo que está a medio subir se borra.
  - Las subidas tus a medias (`tmp/tus/`) se borran tras `BODA_UPLOAD_RESUME_TTL` sin actividad, salvo con un `PATCH` en curso ([ADR-0014](0014-subidas-reanudables-tus.md)).
  - Lo que está en `originals/` sin registro va a `quarantine/`, **nunca se borra**.
- **Consecuencias:** `quarantine/` se revisa a mano antes del `rsync` final.
