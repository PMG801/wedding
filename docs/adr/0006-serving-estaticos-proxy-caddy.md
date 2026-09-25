# ADR-0006 · Serving, streaming y descargas: el proxy sirve directamente

- **Contexto:** hay que entregar miniaturas, fotos y vídeos con peticiones Range sin cargar la aplicación. Lo ocultado debe dejar de estar accesible.
- **Decisión:**
  - Caddy sirve `/m/o/*` (`originals/`) y `/m/d/*` (`derived/`) en solo lectura.
  - Cabeceras: `Cache-Control: public, max-age=31536000, immutable`, `X-Content-Type-Options: nosniff` y **sin compresión**.
  - `Content-Type` explícito para `.heic`, `.mov` y `.mp4`: **hay que comprobar** qué tipo pone Caddy en la imagen final y fijarlo si falta.
  - **Ocultar** es mover el archivo a `hidden/`, que no se sirve, y marcarlo en la base de datos: la URL da 404 al instante.
  - **Descarga:** un enlace del mismo origen con el atributo `download` y el nombre de archivo que proponga la aplicación. De respaldo, `?dl=1`, con el que Caddy añade la cabecera `Content-Disposition: attachment`.
  - El administrador ve lo ocultado a través de la aplicación.
  - En la galería, los vídeos son una imagen de portada. El reproductor solo aparece en la vista de detalle y empieza cargando solo los metadatos.
- **Motivo determinante:** con este tráfico, la aplicación no añade nada. Delegar en el proxy cumple el requisito de "entrega desacoplada" con cero código.
- **Descartado:**
  - Que la aplicación sirva los archivos: cada byte pasaría dos veces por el sistema sin que gane nada.
  - X-Accel-Redirect o `forward_auth`: autorizar cada descarga no aporta valor en un evento familiar.
- **Consecuencias:**
  - Quien tenga una URL puede descargar el archivo mientras sea visible.
  - Lo ya guardado en la caché de un navegador no se puede retirar. Se acepta.
  - Si algún día hiciera falta, se pasa a `forward_auth` sin cambiar el almacenamiento.
