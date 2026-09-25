# ADR-0004 · Miniaturas generadas en el cliente, con respaldo diferido en el servidor

- **Contexto:**
  - La galería necesita imágenes pequeñas que vean todos los navegadores.
  - En el caso típico, las HEIC de iPhone no se ven en Android.
  - Solo hay 2 vCPU.
- **Decisión:**
  1. El cliente decodifica cada foto **una sola vez** y genera dos JPEG: `_t`, de unos 400 px, y `_d`, de unos 2.048 px, respetando la orientación que ya aplica el navegador.
  2. En los vídeos, la portada es un fotograma capturado en el propio dispositivo.
  3. Si el cliente no puede generarlas, sube solo el original y la galería muestra un icono genérico.
  4. **Fase 2**, con `BODA_THUMB_FALLBACK=on`: la aplicación encola esos archivos y un proceso hijo (`vipsthumbnail` o `ffmpeg`) los procesa **de uno en uno**, con prioridad baja y límite de tiempo.
  5. **No hay generación bajo demanda ni transcodificación de vídeo.**
- **Motivo determinante:** el iPhone es el único dispositivo que decodifica de forma eficiente sus propias HEIC y HEVC. Si procesa él, el servidor no gasta CPU.
- **Descartado:**
  - Generarlas bajo demanda: la primera persona que abre la galería dispara decenas de decodificaciones a la vez.
  - Un worker obligatorio en el servidor: CPU y memoria para algo que el cliente ya hace.
  - WebAssembly con libheif: peso y memoria en el móvil.
- **Consecuencias:**
  - El servidor confía en que la miniatura corresponde al original. Se acepta en un contexto familiar.
  - El consumo de batería recae en el invitado; se procesa un archivo cada vez.
  - Sin la fase 2, algunos elementos de Android se ven solo con el icono genérico.
