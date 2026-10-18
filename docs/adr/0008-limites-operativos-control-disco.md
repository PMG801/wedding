# ADR-0008 · Límites operativos y control del disco

- **Decisión:**
  - Los límites de la sección 2.2 se aplican **en el servidor**. El cliente los replica solo para dar mejores mensajes.
  - Por debajo de `BODA_DISK_MIN_FREE_BYTES` de espacio libre, las subidas se desactivan automáticamente y el panel lo avisa.
  - El interruptor manual está en SQLite.
- **Motivo:** los límites sustituyen a la ingeniería defensiva, como acordamos.
- **Reserva de espacio en curso:** `statfs` solo ve los bytes ya escritos. Por eso cada subida (original, vídeo o derivado) reserva en memoria su tamaño declarado antes de leer el cuerpo, y solo se acepta si `libre − reservado − declarado ≥ BODA_DISK_MIN_FREE_BYTES`. La reserva baja a medida que se escriben bytes y se libera al terminar o fallar la subida. Así, 25 vídeos simultáneos no pueden pasar todos la comprobación contra el mismo espacio libre y llenar el volumen. La reserva vive en el proceso: un reinicio la pone a cero, y los clientes cortados reintentan desde el principio.
- **Consecuencias:** con el umbral activo, algún invitado verá "subidas en pausa".
