import type { UploadErrorCode, UploadItem } from '../upload/queue'

/** Guest-facing copy. The UI is Spanish only (S5); edit the names here. */
export const copy = {
  title: 'Nuestra boda',
  subtitle: 'Comparte tus fotos de este día con nosotros',
  pageTitle: 'Nuestra boda · Fotos',
  upload: {
    button: 'Subir fotos',
    hint: 'Puedes elegir varias a la vez. Se suben de dos en dos; no cierres esta página hasta que terminen.',
    retry: 'Reintentar',
    fallback: 'Sin vista previa: se ha subido solo la foto original.',
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
    photoAlt: 'Foto de la boda'
  },
  detail: {
    close: 'Cerrar',
    previous: 'Foto anterior',
    next: 'Foto siguiente',
    download: 'Descargar original',
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
  unsupported_type: 'Solo se admiten fotos JPEG, PNG o HEIC.',
  too_large: 'La foto supera el tamaño máximo de 50 MB.',
  session_expired: 'Tu acceso ha caducado. Vuelve a escanear el código QR y reintenta.',
  storage_full: 'No queda espacio para más fotos ahora mismo. Avísanos, por favor.',
  rejected: 'El servidor no ha aceptado esta foto.',
  network: 'No hay conexión. Comprueba tu cobertura y reintenta.',
  server: 'El servidor está ocupado. Reintenta en un momento.'
}

export function uploadStatusLabel(item: Pick<UploadItem, 'status' | 'progress'>): string {
  switch (item.status) {
    case 'queued':
      return 'En espera'
    case 'preparing':
      return 'Preparando…'
    case 'uploading':
      return `Subiendo… ${Math.round(item.progress * 100)} %`
    case 'retrying':
      return 'Reintentando…'
    case 'done':
      return 'Subida'
    case 'failed':
      return 'No se ha podido subir'
  }
}
