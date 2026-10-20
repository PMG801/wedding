import type { DraftErrorCode } from '../pinned/draft'

/** Administrator copy (Spanish only). Kept apart from the guest copy in es.ts. */
export const adminCopy = {
  title: 'Administración',
  pageTitle: 'Nuestra boda · Administración',
  loading: 'Cargando…',
  unavailable: 'No podemos conectar con el servidor. Inténtalo de nuevo en unos minutos.',
  error: 'No se ha podido actualizar. Inténtalo de nuevo.',
  login: {
    heading: 'Entrar como administrador',
    password: 'Contraseña',
    submit: 'Entrar',
    submitting: 'Comprobando…',
    invalid: 'Contraseña incorrecta. Si fallas varias veces seguidas, la web tardará más en responder.',
    failed: 'No se ha podido iniciar sesión. Inténtalo de nuevo en unos minutos.'
  },
  uploads: {
    heading: 'Subidas',
    open: 'Subidas abiertas',
    paused: 'Subidas en pausa',
    pause: 'Pausar subidas',
    resume: 'Reanudar subidas'
  },
  disk: {
    heading: 'Disco',
    autoPaused:
      'Queda poco espacio en el disco: las subidas se pausan automáticamente hasta que se libere espacio.',
    free: 'Espacio libre',
    reserved: 'Reservado por subidas en curso'
  },
  moderation: {
    heading: 'Ocultas',
    empty: 'No hay nada oculto.',
    thumbAlt: 'Foto oculta',
    noPreview: 'Sin vista previa',
    show: 'Mostrar',
    loadMore: 'Cargar más',
    loadFailed: 'No se han podido cargar los medios ocultos. Inténtalo de nuevo.',
    failed: 'No se ha podido mostrar el medio. Inténtalo de nuevo.'
  },
  logout: 'Cerrar sesión',
pinned: {
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
} as const

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
