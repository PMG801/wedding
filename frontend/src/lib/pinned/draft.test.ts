import { describe, expect, it } from 'vitest'
import { BODY_MAX, TITLE_MAX, validatePinnedDraft } from './draft'

const uuid = '123e4567-e89b-42d3-a456-426614174000'

describe('validatePinnedDraft', () => {
  it('accepts a valid draft, trims the title and keeps the body as typed', () => {
    expect(validatePinnedDraft({ title: '  Horario  ', body: ' 13:00\n14:30 ', mediaId: null })).toEqual({
      ok: true,
      draft: { title: 'Horario', body: ' 13:00\n14:30 ', mediaId: null }
    })
  })

  it('requires a title with at least one character after trimming', () => {
    expect(validatePinnedDraft({ title: '   ', body: '', mediaId: null })).toEqual({
      ok: false,
      errors: [{ field: 'title', code: 'title_required' }]
    })
  })

  it('counts the title in code points, not UTF-16 units', () => {
    const exactly = '😀'.repeat(TITLE_MAX)
    expect(validatePinnedDraft({ title: exactly, body: '', mediaId: null }).ok).toBe(true)

    const over = '😀'.repeat(TITLE_MAX + 1)
    expect(validatePinnedDraft({ title: over, body: '', mediaId: null })).toEqual({
      ok: false,
      errors: [{ field: 'title', code: 'title_too_long' }]
    })
  })

  it('rejects control characters in the title, including newlines', () => {
    expect(validatePinnedDraft({ title: 'Hola\nmundo', body: '', mediaId: null })).toEqual({
      ok: false,
      errors: [{ field: 'title', code: 'title_control' }]
    })
  })

  it('allows an empty body and up to 2000 code points', () => {
    expect(validatePinnedDraft({ title: 'T', body: '', mediaId: null }).ok).toBe(true)
    expect(validatePinnedDraft({ title: 'T', body: '😀'.repeat(BODY_MAX), mediaId: null }).ok).toBe(true)
    expect(validatePinnedDraft({ title: 'T', body: 'a'.repeat(BODY_MAX + 1), mediaId: null })).toEqual({
      ok: false,
      errors: [{ field: 'body', code: 'body_too_long' }]
    })
  })

  it('allows newlines in the body but no other control characters', () => {
    expect(validatePinnedDraft({ title: 'T', body: 'a\nb', mediaId: null }).ok).toBe(true)
    for (const bad of ['a\rb', 'a\tb', 'a\u0000b', 'a\u007fb']) {
      expect(validatePinnedDraft({ title: 'T', body: bad, mediaId: null })).toEqual({
        ok: false,
        errors: [{ field: 'body', code: 'body_control' }]
      })
    }
  })

  it('treats an empty media id as no image and lowercases a valid one', () => {
    expect(validatePinnedDraft({ title: 'T', body: '', mediaId: '  ' })).toEqual({
      ok: true,
      draft: { title: 'T', body: '', mediaId: null }
    })
    expect(validatePinnedDraft({ title: 'T', body: '', mediaId: uuid.toUpperCase() })).toEqual({
      ok: true,
      draft: { title: 'T', body: '', mediaId: uuid }
    })
  })

  it('rejects a media id that is not a UUIDv4', () => {
    expect(validatePinnedDraft({ title: 'T', body: '', mediaId: 'foto-1' })).toEqual({
      ok: false,
      errors: [{ field: 'mediaId', code: 'media_id_invalid' }]
    })
    expect(validatePinnedDraft({ title: 'T', body: '', mediaId: '123e4567-e89b-12d3-a456-426614174000' }).ok).toBe(false)
  })

  it('rejects a JSON payload above 8 KiB even when both fields are within their limits', () => {
    const result = validatePinnedDraft({ title: '😀'.repeat(TITLE_MAX), body: '😀'.repeat(BODY_MAX), mediaId: null })
    expect(result).toEqual({ ok: false, errors: [{ field: 'form', code: 'payload_too_large' }] })
  })
})
