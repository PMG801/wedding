import { describe, it, expect } from 'vitest'
import { greeting } from './greeting'

describe('greeting', () => {
  it('greets a named person', () => {
    expect(greeting('Ada')).toBe('Hello, Ada!')
  })

  it('trims surrounding whitespace', () => {
    expect(greeting('  Bob  ')).toBe('Hello, Bob!')
  })

  it('falls back for empty input', () => {
    expect(greeting('')).toBe('Hello, stranger!')
    expect(greeting('   ')).toBe('Hello, stranger!')
  })
})
