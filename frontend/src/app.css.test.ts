import { describe, expect, it } from 'vitest'
import css from './app.css?raw'

/** The declarations of the first rule whose selector is exactly `selector`. */
function declarations(selector: string): Map<string, string> {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const match = new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`, 'm').exec(css)
  if (match === null) throw new Error(`rule ${selector} not found`)
  const result = new Map<string, string>()
  for (const line of match[1].split(';')) {
    const colon = line.indexOf(':')
    if (colon > 0) result.set(line.slice(0, colon).trim(), line.slice(colon + 1).trim())
  }
  return result
}

/** Resolves `#rrggbb`, `#fff` or `var(--token)` to a six-digit hex colour. */
function colour(value: string, tokens: Map<string, string>): string {
  const reference = /^var\((--[\w-]+)\)$/.exec(value)
  if (reference !== null) {
    const token = tokens.get(reference[1])
    if (token === undefined) throw new Error(`unknown token ${reference[1]}`)
    return colour(token, tokens)
  }
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(value)
  if (short !== null) return `#${short[1]}${short[1]}${short[2]}${short[2]}${short[3]}${short[3]}`
  if (/^#[0-9a-f]{6}$/i.test(value)) return value
  throw new Error(`unsupported colour ${value}`)
}

function luminance(hex: string): number {
  const channel = (offset: number) => {
    const value = parseInt(hex.slice(offset, offset + 2), 16) / 255
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * channel(1) + 0.7152 * channel(3) + 0.0722 * channel(5)
}

function contrast(foreground: string, background: string): number {
  const [lighter, darker] = [luminance(foreground), luminance(background)].sort((a, b) => b - a)
  return (lighter + 0.05) / (darker + 0.05)
}

describe('button colours', () => {
  const tokens = declarations(':root')

  it('keeps the text of the primary button at WCAG AA contrast (4.5:1)', () => {
    const primary = declarations('.button.primary')
    const ratio = contrast(colour(primary.get('color')!, tokens), colour(primary.get('background')!, tokens))
    expect(ratio).toBeGreaterThanOrEqual(4.5)
  })

  it('keeps the text of the default button at WCAG AA contrast (4.5:1)', () => {
    const button = declarations('.button')
    const ratio = contrast(colour(button.get('color')!, tokens), colour(button.get('background')!, tokens))
    expect(ratio).toBeGreaterThanOrEqual(4.5)
  })
})
