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
  logout: 'Cerrar sesión'
} as const
