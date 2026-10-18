import { GuestSessionRequiredError, type GalleryItem, type GalleryPage } from '../api/media'

export interface GalleryFeedState {
  items: GalleryItem[]
  status: 'idle' | 'loading' | 'error' | 'session_required'
  hasMore: boolean
}

export interface GalleryFeedOptions {
  fetchPage: (cursor: string | null) => Promise<GalleryPage>
  onChange: (state: GalleryFeedState) => void
}

/** Newest-first gallery with cursor pagination and head refresh after uploads. */
export class GalleryFeed {
  readonly #options: GalleryFeedOptions
  #items: GalleryItem[] = []
  #cursor: string | null = null
  #hasMore = true
  #status: GalleryFeedState['status'] = 'idle'

  constructor(options: GalleryFeedOptions) {
    this.#options = options
  }

  get state(): GalleryFeedState {
    return { items: [...this.#items], status: this.#status, hasMore: this.#hasMore }
  }

  /** Loads the next older page; does nothing while loading or at the end. */
  async loadMore(): Promise<void> {
    if (this.#status === 'loading' || !this.#hasMore) return
    const page = await this.#fetch(this.#cursor)
    if (page === null) return
    const known = new Set(this.#items.map((entry) => entry.id))
    this.#items.push(...page.items.filter((entry) => !known.has(entry.id)))
    this.#cursor = page.nextCursor
    this.#hasMore = page.nextCursor !== null
    this.#emit()
  }

  /** Drops one photo from the list (e.g. after it was hidden). Does not refetch. */
  remove(id: string): void {
    const remaining = this.#items.filter((entry) => entry.id !== id)
    if (remaining.length === this.#items.length) return
    this.#items = remaining
    this.#emit()
  }

  /** Fetches the newest page and merges it on top, e.g. after an upload finishes. */
  async refresh(): Promise<void> {
    if (this.#status === 'loading') return
    const page = await this.#fetch(null)
    if (page === null) return
    const fresh = new Map(page.items.map((entry) => [entry.id, entry]))
    const overlaps = this.#items.some((entry) => fresh.has(entry.id))
    if (this.#items.length === 0 || !overlaps) {
      // Too many new photos to stitch the pages together: start over.
      this.#items = page.items
      this.#cursor = page.nextCursor
      this.#hasMore = page.nextCursor !== null
    } else {
      const known = new Set(this.#items.map((entry) => entry.id))
      const added = page.items.filter((entry) => !known.has(entry.id))
      // Shown photos may have gained derivative URLs since they were listed.
      this.#items = [...added, ...this.#items.map((entry) => fresh.get(entry.id) ?? entry)]
    }
    this.#emit()
  }

  async #fetch(cursor: string | null): Promise<GalleryPage | null> {
    this.#status = 'loading'
    this.#emit()
    try {
      const page = await this.#options.fetchPage(cursor)
      this.#status = 'idle'
      return page
    } catch (error) {
      this.#status = error instanceof GuestSessionRequiredError ? 'session_required' : 'error'
      this.#emit()
      return null
    }
  }

  #emit(): void {
    this.#options.onChange(this.state)
  }
}
