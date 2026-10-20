import type { UploadErrorCode } from '../upload/queue'

/** Guest-facing copy. The UI is Spanish only (S5); edit the names here. */
export const copy = {
  title: 'Nuestra boda',
  subtitle: 'Comparte tus fotos de este día con nosotros',
  pageTitle: 'Nuestra boda · Fotos',
  upload: {
    button: 'Subir fotos y vídeos',
    navLabel: 'Subir',
    keepOpen: 'No cierres esta página hasta que terminen.',
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
    videoLabel: 'Vídeo',
    viewsLabel: 'Orden de la galería',
    views: { recent: 'Recientes', top: 'Más gustadas' }
  },
  ranking: {
    empty: 'Todavía no hay fotos con me gusta'
  },
  pinned: {
    heading: 'Fijados',
    show: 'Mostrar fijados',
    hide: 'Ocultar fijados',
    readMore: 'Leer más',
    readLess: 'Leer menos',
    imageAlt: 'Foto de la publicación fijada',
    openImage: 'Ampliar la foto'
  },
  likes: {
    like: 'Me gusta',
    unlike: 'Quitar me gusta',
    failed: 'No se ha podido guardar tu me gusta. Inténtalo de nuevo.'
  },
  detail: {
    close: 'Cerrar',
    previous: 'Foto anterior',
    next: 'Foto siguiente',
    download: 'Descargar',
    videoUnsupported: 'Tu navegador no puede reproducir este vídeo. Puedes descargar el original.',
    noPreview: 'Este formato no se puede mostrar en tu navegador. Puedes descargar la foto original.',
    hide: 'Ocultar',
    hideConfirm:
      'Los invitados dejarán de ver esta foto. No se borra: puedes volver a mostrarla desde la página de administración.',
    hideFailed: 'No se ha podido ocultar la foto. Inténtalo de nuevo.'
  },
  entry: {
    heading: 'Escanea el código QR',
    body: 'Para ver y compartir las fotos, abre la web escaneando el código QR de la invitación o de las mesas.',
    unavailable: 'No podemos conectar con el servidor. Inténtalo de nuevo en unos minutos.',
    checking: 'Comprobando el acceso…'
  }
} as const

/**
 * Accessible name of a like button: the action plus the photo's like count (the count is not visible in the viewer's text).
 * The `count` argument is the current count, before the tap.
 */
export const likeLabels = {
  button: (liked: boolean, count: number): string => {
    const action = liked ? copy.likes.unlike : copy.likes.like
    return `${action}, ${count} ${count === 1 ? 'me gusta' : 'me gustas'}`
  }
}

/** Shown when the admin has paused uploads (503 uploads_paused). Retry is manual. */
export const uploadPausedMessage = 'Las subidas están en pausa'

export const uploadErrorMessages: Record<Exclude<UploadErrorCode, 'uploads_paused'>, string> = {
  unsupported_type: 'Solo se admiten fotos JPEG, PNG o HEIC y vídeos MP4 o MOV.',
  too_large: 'El archivo supera el tamaño máximo (50 MB en fotos y 1,5 GB en vídeos).',
  session_expired: 'Tu acceso ha caducado. Vuelve a escanear el código QR y reintenta.',
  storage_full: 'No queda espacio para más fotos ahora mismo. Avísanos, por favor.',
  rejected: 'El servidor no ha aceptado esta foto.',
  network: 'No hay conexión. Comprueba tu cobertura y reintenta.',
  server: 'El servidor está ocupado. Reintenta en un momento.'
}

export function uploadErrorMessage(code: UploadErrorCode): string {
  return code === 'uploads_paused' ? uploadPausedMessage : uploadErrorMessages[code]
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
