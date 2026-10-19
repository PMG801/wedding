import { describe, expect, it } from 'vitest'
import { adoptDeepLink, closeViewer, moveViewer, openViewer, type HistoryLike } from './history'

/** Minimal session history: entries, a cursor, and back() that moves the cursor. */
class FakeHistory implements HistoryLike {
  entries: { state: unknown; url: string }[]
  index = 0

  constructor(url: string, state: unknown = null) {
    this.entries = [{ state, url }]
  }

  get state(): unknown {
    return this.entries[this.index].state
  }

  get url(): string {
    return this.entries[this.index].url
  }

  pushState(state: unknown, _unused: string, url: string): void {
    this.entries = [...this.entries.slice(0, this.index + 1), { state, url }]
    this.index += 1
  }

  replaceState(state: unknown, _unused: string, url: string): void {
    this.entries[this.index] = { state, url }
  }

  back(): void {
    if (this.index > 0) this.index -= 1
  }
}

describe('viewer history (S4)', () => {
  it('opening a photo pushes one entry, so the back button returns to the gallery', () => {
    const history = new FakeHistory('/')
    openViewer(history, '#/foto/a')
    expect(history.entries).toHaveLength(2)
    expect(history.url).toBe('#/foto/a')
    history.back()
    expect(history.url).toBe('/')
  })

  it('moving between photos replaces the entry and keeps the viewer marker', () => {
    const history = new FakeHistory('/')
    openViewer(history, '#/foto/a')
    moveViewer(history, '#/foto/b')
    moveViewer(history, '#/foto/c')
    expect(history.entries).toHaveLength(2)
    expect(history.url).toBe('#/foto/c')
    expect(closeViewer(history, '/')).toBe('back')
    expect(history.url).toBe('/')
  })

  it('closing a viewer opened in the app goes back, leaving no extra entry', () => {
    const history = new FakeHistory('/')
    openViewer(history, '#/foto/a')
    expect(closeViewer(history, '/')).toBe('back')
    expect(history.index).toBe(0)
    expect(history.url).toBe('/')
  })

  it('closing a viewer without the marker replaces the entry instead of leaving the page', () => {
    const history = new FakeHistory('#/foto/a')
    expect(closeViewer(history, '/')).toBe('replaced')
    expect(history.entries).toEqual([{ state: null, url: '/' }])
  })

  it('a deep link gets a gallery entry underneath, so back closes the photo', () => {
    const history = new FakeHistory('/?x=1#/foto/a')
    adoptDeepLink(history, '/?x=1', '#/foto/a')
    expect(history.entries.map((entry) => entry.url)).toEqual(['/?x=1', '#/foto/a'])
    expect(closeViewer(history, '/?x=1')).toBe('back')
    expect(history.url).toBe('/?x=1')
  })

  it('a reload of an already adopted viewer entry adds nothing', () => {
    const history = new FakeHistory('/')
    openViewer(history, '#/foto/a')
    adoptDeepLink(history, '/', '#/foto/a')
    expect(history.entries).toHaveLength(2)
  })
})
