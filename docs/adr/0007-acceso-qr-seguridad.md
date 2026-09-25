# ADR-0007 · Acceso y seguridad mínima

- **Contexto:** acceso sin registro por QR y un único administrador. No hay amenazas serias.
- **Decisión:**
  - El QR apunta a `/e/{token}`. La aplicación comprueba el token, crea una cookie de invitado firmada (HttpOnly, Secure, SameSite=Lax, 30 días) y **redirige a `/`**, para que el token no se quede en la barra de direcciones ni salga en capturas de pantalla.
  - El administrador entra con una contraseña con hash y una cookie de sesión aparte (SameSite=Strict).
  - Login con un retraso progresivo tras fallos.
  - Las rutas `/api/admin/*` exigen la sesión del administrador.
  - Solo se aceptan JPEG, HEIC, HEIF, PNG, MP4 y MOV, comprobando la firma del archivo, lo que excluye SVG y HTML.
- **Motivo:** es la seguridad mínima razonable sin añadir fricción.
- **Descartado:** cuentas por invitado, OAuth y tokens por mesa.
- **Consecuencias:** si el token se filtra, se cambia y se reimprime el QR; se acepta.
