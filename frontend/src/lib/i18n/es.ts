import type { UploadErrorCode } from '../upload/queue'

/** Guest-facing copy. The UI is Spanish only (S5); edit the names here. */
export const copy = {
  title: 'Nuestra boda',
  subtitle: 'Comparte tus fotos de este día con nosotros',
  pageTitle: 'Nuestra boda · Fotos',
  upload: {
    button: 'Subir fotos y vídeos',
    hint: 'Puedes elegir varias a la vez. Las fotos se suben de dos en dos y cada vídeo va solo, así que pueden tardar; no cierres esta página hasta que terminen.',
    retry: 'Reintentar',
    progress: 'Progreso de la subida',
    clearDone: 'Ocultar las subidas terminadas'
  },
  gallery: {
    heading: 'Galería',
    empty: 'Todavía no hay fotos. ¡Sé el primero en compartir una!',
    loadMore: 'Ver más fotos',
    loading: 'Cargando fotos…',
    error: 'No se han podido cargar las fotos.',
    retry: 'Volver a intentarlo',
    end: 'Ya has visto todas las fotos.',
    noPreview: 'Vista previa no disponible',
    photoAlt: 'Foto de la boda',
    videoLabel: 'Vídeo'
  },
  detail: {
    close: 'Cerrar',
    previous: 'Foto anterior',
    next: 'Foto siguiente',
    download: 'Descargar original',
    videoUnsupported: 'Tu navegador no puede reproducir este vídeo. Puedes descargar el original.',
    noPreview: 'Este formato no se puede mostrar en tu navegador. Puedes descargar la foto original.'
  },
  entry: {
    heading: 'Escanea el código QR',
    body: 'Para ver y compartir las fotos, abre la web escaneando el código QR de la invitación o de las mesas.',
    unavailable: 'No podemos conectar con el servidor. Inténtalo de nuevo en unos minutos.',
    checking: 'Comprobando el acceso…'
  }
} as const

export const uploadErrorMessages: Record<UploadErrorCode, string> = {
  unsupported_type: 'Solo se admiten fotos JPEG, PNG o HEIC y vídeos MP4 o MOV.',
  too_large: 'El archivo supera el tamaño máximo (50 MB en fotos y 1,5 GB en vídeos).',
  session_expired: 'Tu acceso ha caducado. Vuelve a escanear el código QR y reintenta.',
  storage_full: 'No queda espacio para más fotos ahora mismo. Avísanos, por favor.',
  rejected: 'El servidor no ha aceptado esta foto.',
  network: 'No hay conexión. Comprueba tu cobertura y reintenta.',
  server: 'El servidor está ocupado. Reintenta en un momento.'
}

export const uploadSummaryLabels = {
  remaining: (remaining: number, total: number): string =>
    remaining === 1 ? 'Queda 1 foto por subir' : `Quedan ${remaining} de ${total} fotos por subir`,
  finished: (done: number): string => (done === 1 ? '✓ 1 foto subida' : `✓ ${done} fotos subidas`),
  failed: (count: number): string =>
    count === 1 ? 'Una foto no se ha podido subir' : `${count} fotos no se han podido subir`,
  rejected: (count: number): string =>
    `${count === 1 ? 'Un archivo no se puede subir' : `${count} archivos no se pueden subir`}: solo se admiten fotos JPEG, PNG o HEIC de hasta 50 MB y vídeos MP4 o MOV de hasta 1,5 GB.`
}
