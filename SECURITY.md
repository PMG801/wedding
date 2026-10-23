# Política de seguridad

## Versiones con soporte

Solo se mantiene la última versión publicada y la rama `develop`.

## Cómo informar de una vulnerabilidad

**No abras un issue público.** Usa el aviso privado de GitHub: en la pestaña **Security** del repositorio, pulsa **Report a vulnerability**.

Incluye, si puedes:

- una descripción del problema y su impacto;
- los pasos para reproducirlo;
- la versión o el commit afectado.

Se confirmará la recepción lo antes posible y se te mantendrá informado de la corrección. Cuando esté publicada, se te mencionará en la nota de la release si lo deseas.

## Alcance

Son de especial interés los fallos que permitan:

- acceder a los medios o a la administración sin sesión válida;
- saltarse el límite de intentos del login de administrador;
- subir contenido que el servidor sirva con un tipo incorrecto o ejecute;
- leer o escribir fuera del directorio de datos.

## Para quien despliega

- Genera el token del evento, la clave de sesión y el hash de la contraseña de administrador con valores aleatorios propios; nunca uses los de `.env.example`.
- No subas `.env` ni `.admin-password` al repositorio.
- Si el token del QR se filtra, cámbialo y reimprime el QR (ver [ADR-0007](docs/adr/0007-acceso-qr-seguridad.md)).
