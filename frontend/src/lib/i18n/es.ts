import type { UploadErrorCode } from '../upload/queue'
import type { DraftErrorCode } from '../pinned/draft'

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
    videoLabel: 'Vídeo',
    viewsLabel: 'Orden de la galería',
    views: { recent: 'Recientes', top: 'Más gustadas' }
  },
  ranking: {
    empty: 'Todavía no hay fotos con me gusta'
  },
  pinned: {
    heading: 'Fijados',
    expand: 'Desplegar',
    collapse: 'Plegar',
    imageAlt: 'Foto de la publicación fijada',
    admin: {
      heading: 'Publicaciones fijadas',
      intro: 'Hasta 5 publicaciones encima de la galería. Solo texto: los saltos de línea se conservan y no se admite formato.',
      empty: 'Todavía no hay publicaciones fijadas.',
      loadFailed: 'No se han podido cargar las publicaciones fijadas.',
      failed: 'No se ha podido guardar la publicación. Inténtalo de nuevo.',
      deleteFailed: 'No se ha podido borrar la publicación. Inténtalo de nuevo.',
      limit: 'Ya hay 5 publicaciones fijadas. Borra una para añadir otra.',
      invalid: 'El servidor no ha aceptado la publicación. Revisa los campos.',
      stale: 'Esta publicación ya no existe. La lista se ha actualizado.',
      orderFailed: 'No se ha podido guardar el orden. La lista se ha actualizado.',
      moveUp: 'Subir',
      moveDown: 'Bajar',
      edit: 'Editar',
      delete: 'Borrar',
      confirmDelete: 'Confirmar borrado',
      cancel: 'Cancelar',
      deleteQuestion: '¿Borrar esta publicación? No se puede deshacer.',
      createHeading: 'Nueva publicación',
      editHeading: 'Editar publicación',
      titleLabel: 'Título',
      bodyLabel: 'Texto',
      mediaLabel: 'Identificador de la foto (opcional)',
      mediaHint: 'Abre la foto en la galería y copia el identificador que aparece en la dirección después de «#/foto/». Déjalo vacío para no poner imagen.',
      create: 'Añadir',
      save: 'Guardar cambios',
      saving: 'Guardando…',
      noImage: 'Sin imagen',
      hidden: 'La foto está oculta: los invitados verán la publicación sin imagen.'
    }
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
    download: 'Descargar original',
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

/** Shown under the editor field that failed client-side validation (mirrors ADR-0013, D7). */
export const pinnedDraftErrorMessages: Record<DraftErrorCode, string> = {
  title_required: 'Escribe un título.',
  title_too_long: 'El título no puede superar 120 caracteres.',
  title_control: 'El título no admite saltos de línea ni caracteres especiales.',
  body_too_long: 'El texto no puede superar 2000 caracteres.',
  body_control: 'El texto solo admite saltos de línea.',
  media_id_invalid: 'El identificador de la foto no es válido.',
  payload_too_large: 'La publicación es demasiado larga. Acórtala un poco.'
}
