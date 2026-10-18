import type { PinnedDraft } from '../api/pinned'

/** Client mirror of the D7 limits (ADR-0013). The server stays the authority; this only gives earlier messages. */
export const TITLE_MAX = 120
export const BODY_MAX = 2000
/** Maximum JSON body the server accepts for a pinned entry (8 KiB). */
export const PAYLOAD_MAX_BYTES = 8 * 1024

export type DraftField = 'title' | 'body' | 'mediaId' | 'form'

export type DraftErrorCode =
  | 'title_required'
  | 'title_too_long'
  | 'title_control'
  | 'body_too_long'
  | 'body_control'
  | 'media_id_invalid'
  | 'payload_too_large'

export interface DraftFailure {
  field: DraftField
  code: DraftErrorCode
}

export type DraftResult = { ok: true; draft: PinnedDraft } | { ok: false; errors: DraftFailure[] }

/** Canonical lowercase UUIDv4, the only shape of media_id the server accepts. */
const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

/** Any Unicode control character (category Cc). */
const controlCharacter = /\p{Cc}/u

/**
 * Validates an editor draft before sending it. The title is trimmed first; the body is sent as typed.
 * Lengths are counted in code points. An empty media id means no image.
 */
export function validatePinnedDraft(input: PinnedDraft): DraftResult {
  const errors: DraftFailure[] = []
  const title = input.title.trim()
  const titleLength = [...title].length
  const body = input.body
  const mediaText = (input.mediaId ?? '').trim().toLowerCase()
  const mediaId = mediaText === '' ? null : mediaText

  if (titleLength === 0) errors.push({ field: 'title', code: 'title_required' })
  else if (controlCharacter.test(title)) errors.push({ field: 'title', code: 'title_control' })
  else if (titleLength > TITLE_MAX) errors.push({ field: 'title', code: 'title_too_long' })

  // The body allows line feeds and nothing else from the control range.
  if (controlCharacter.test(body.replaceAll('\n', ''))) errors.push({ field: 'body', code: 'body_control' })
  else if ([...body].length > BODY_MAX) errors.push({ field: 'body', code: 'body_too_long' })

  if (mediaId !== null && !uuidV4.test(mediaId)) errors.push({ field: 'mediaId', code: 'media_id_invalid' })

  const draft: PinnedDraft = { title, body, mediaId }
  if (errors.length === 0 && payloadBytes(draft) > PAYLOAD_MAX_BYTES) {
    errors.push({ field: 'form', code: 'payload_too_large' })
  }
  return errors.length === 0 ? { ok: true, draft } : { ok: false, errors }
}

function payloadBytes(draft: PinnedDraft): number {
  return new TextEncoder().encode(
    JSON.stringify({ title: draft.title, body: draft.body, media_id: draft.mediaId })
  ).length
}
