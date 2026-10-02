# Registros de decisiones de arquitectura

Los ADR documentan decisiones arquitectónicas duraderas. Se nombran `NNNN-slug.md`, con un número de cuatro cifras y un slug breve en español. No se reutilizan números.

## Estados

Indica el estado al comienzo del ADR: **Propuesto**, **Aceptado**, **Sustituido** o **Retirado**. Un ADR sustituido debe enlazar el ADR que lo reemplaza; uno retirado explica por qué deja de aplicar. Los ADR existentes 0001–0010 se consideran aceptados.

## Índice

- [ADR-0001 · Topología: monolito con proxy (Opción A)](0001-topologia-monolito-proxy.md)
- [ADR-0002 · Almacenamiento de medios en el disco local](0002-almacenamiento-medios-disco-local.md)
- [ADR-0003 · Motor de datos: SQLite en modo WAL](0003-motor-datos-sqlite-wal.md)
- [ADR-0004 · Miniaturas generadas en el cliente, con respaldo diferido en el servidor](0004-miniaturas-cliente-respaldo-diferido.md)
- [ADR-0005 · Subida con una petición por archivo, reintento idempotente y criterio para pasar a TUS](0005-subida-streaming-idempotente.md)
- [ADR-0006 · Serving, streaming y descargas: el proxy sirve directamente](0006-serving-estaticos-proxy-caddy.md)
- [ADR-0007 · Acceso y seguridad mínima](0007-acceso-qr-seguridad.md)
- [ADR-0008 · Límites operativos y control del disco](0008-limites-operativos-control-disco.md)
- [ADR-0009 · Limpieza y conciliación](0009-limpieza-conciliacion-huerfanos.md)
- [ADR-0010 · Entrega y copia: fuera de la aplicación](0010-entrega-rsync-fuera-aplicacion.md)
