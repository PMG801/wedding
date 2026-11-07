import { describe, expect, it } from 'vitest'
import appCss from './app.css?raw'

const sources = import.meta.glob('./**/*.svelte', { query: '?raw', import: 'default', eager: true }) as Record<string, string>

function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '')
}

/** The CSS of every component `<style>` block, plus app.css without its `:root` token block. */
function stylesheets(): Record<string, string> {
  const result: Record<string, string> = { './app.css': stripComments(appCss).replace(/:root\s*\{[^}]*\}/, '') }
  for (const [path, source] of Object.entries(sources)) {
    const match = /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)
    if (match !== null) result[path] = stripComments(match[1])
  }
  return result
}

/** Font sizes that deliberately sit outside the type scale, by file and exact value. */
const fontSizeExceptions: Record<string, string[]> = {
  './app.css': ['1.9rem', 'clamp(2.6rem, 11vw, 4.2rem)'], // h2 and the hero title
  './components/PhotoDialog.svelte': ['6rem'] // the heart burst
}

function declarations(css: string, property: string): string[] {
  return [...css.matchAll(new RegExp(`(?:^|[;{\\s])${property}\\s*:\\s*([^;}]+)`, 'g'))].map((m) => m[1].trim())
}

describe('style tokens (S8)', () => {
  it('finds the components and app.css', () => {
    expect(Object.keys(stylesheets()).length).toBeGreaterThan(10)
  })

  it('uses no colour literal outside the :root tokens', () => {
    const literal = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?)\(|(?:^|[\s:(,])(?:white|black)(?![-\w])/m
    const offenders = Object.entries(stylesheets()).filter(([, css]) => literal.test(css)).map(([path]) => path)
    expect(offenders).toEqual([])
  })

  it('uses a type-scale token for every font-size, except the named ones', () => {
    const offenders: string[] = []
    for (const [path, css] of Object.entries(stylesheets())) {
      for (const value of declarations(css, 'font-size')) {
        if (!/^var\(--text-(xs|sm|md|base|lg|xl)\)$/.test(value) && !(fontSizeExceptions[path] ?? []).includes(value)) {
          offenders.push(`${path}: ${value}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('keeps the scale at 0.75rem or above', () => {
    for (const [, value] of appCss.matchAll(/--text-[a-z]+:\s*([\d.]+)rem/g)) {
      expect(Number(value)).toBeGreaterThanOrEqual(0.75)
    }
  })

  it('uses a z-index token everywhere', () => {
    const offenders: string[] = []
    for (const [path, css] of Object.entries(stylesheets())) {
      for (const value of declarations(css, 'z-index')) {
        if (!/^var\(--z-[a-z-]+\)$/.test(value)) offenders.push(`${path}: ${value}`)
      }
    }
    expect(offenders).toEqual([])
  })
})

describe('focus and motion (S9)', () => {
  it('has one global focus ring and one global reduced-motion rule', () => {
    expect(appCss).toMatch(/(^|\n)\s*:focus-visible\s*\{[^}]*outline:\s*3px solid var\(--sage-dark\)[^}]*outline-offset:\s*2px/)
    expect(appCss).toMatch(/@media \(prefers-reduced-motion: reduce\)\s*\{\s*\*,\s*\*::before,\s*\*::after\s*\{[^}]*animation-duration:[^}]*transition-duration:/)
  })

  it('gives the dark viewers a light ring', () => {
    for (const file of ['MediaLightbox', 'StoryViewer']) {
      const source = sources[`./components/${file}.svelte`]
      expect(source, file).toMatch(/:focus-visible[^{]*\{[^}]*outline-color:\s*var\(--on-dark\)/)
    }
  })
})
