import { describe, expect, it } from 'vitest'
import { copy } from '../i18n/es'
import { periodLabel } from './label'

describe('periodLabel', () => {
  it('shows the wall-clock range of a period as HH:MM–HH:MM', () => {
    expect(periodLabel('2026-10-18T18:00:00', '2026-10-18T20:00:00')).toBe('18:00–20:00')
  })

  it('accepts the configuration form without seconds', () => {
    expect(periodLabel('2026-10-18T09:30', '2026-10-18T10:15')).toBe('09:30–10:15')
  })

  it('returns null for a missing, malformed or impossible time', () => {
    expect(periodLabel('', '2026-10-18T20:00:00')).toBeNull()
    expect(periodLabel('2026-10-18T18:00:00', 'tarde')).toBeNull()
    expect(periodLabel('2026-10-18T25:00:00', '2026-10-18T26:00:00')).toBeNull()
    expect(periodLabel('2026-10-18T18:61:00', '2026-10-18T20:00:00')).toBeNull()
  })

  it('names the bubble in Spanish', () => {
    expect(copy.carousel.bubble('18:00–20:00')).toContain('18:00–20:00')
  })
})
