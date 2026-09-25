# ADR-0008 · Límites operativos y control del disco

- **Decisión:**
  - Los límites de la sección 2.2 se aplican **en el servidor**. El cliente los replica solo para dar mejores mensajes.
  - Por debajo de `BODA_DISK_MIN_FREE_BYTES` de espacio libre, las subidas se desactivan automáticamente y el panel lo avisa.
  - El interruptor manual está en SQLite.
- **Motivo:** los límites sustituyen a la ingeniería defensiva, como acordamos.
- **Consecuencias:** con el umbral activo, algún invitado verá "subidas en pausa".
