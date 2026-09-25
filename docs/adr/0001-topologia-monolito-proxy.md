# ADR-0001 · Topología: monolito con proxy (Opción A)

- **Contexto:**
  - Un único evento, entre 80 y 110 invitados y concurrencia baja.
  - 2 vCPU ARM64, 12 GB de RAM y un solo operador.
  - El valor está en que funcione el día señalado, no en poder escalar.
- **Decisión:**
  - Dos procesos: Caddy (TLS y archivos) y una aplicación Go (API, subidas, tareas y SQLite embebida).
  - El único trabajo pesado se ejecuta como proceso hijo temporal.
- **Motivo determinante:** es lo mínimo que cumple todos los flujos. Además, si la aplicación cae, Caddy **sigue sirviendo** los archivos ya publicados.
- **Descartado:** la Opción B (API, servidor TUS, worker, cola, MinIO y PostgreSQL): más piezas que operar sin beneficio a esta escala.
- **Consecuencias:**
  - Un fallo de la aplicación corta a la vez las subidas y la API.
  - No escala horizontalmente, y se acepta.
