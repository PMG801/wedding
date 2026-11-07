import { describe, expect, it } from 'vitest'
import css from './app.css?raw'

describe('buttons (S10)', () => {
  it('keeps .button.small at least 44px (2.75rem) tall', () => {
    const block = /\.button\.small\s*\{([^}]*)\}/.exec(css)
    expect(block).not.toBeNull()
    const height = /min-height:\s*([\d.]+)rem/.exec(block![1])
    expect(Number(height?.[1])).toBeGreaterThanOrEqual(2.75)
  })
})
