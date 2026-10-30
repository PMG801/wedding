import { GuestSessionRequiredError, type GalleryItem, type GalleryPage, type LikeResult } from '../api/media'

export type GalleryView = 'recent' | 'top'
/** Direction of the chronological feed; the ranking view has no order. */
export type GalleryOrder = 'newest' | 'oldest'
export type FeedStatus = 'idle' | 'loading' | 'error' | 'session_required'

export interface GalleryFeedState {
  /** Which list `items` shows: the chronological feed or the most-liked ranking. */
  view: GalleryView
  /** Direction of the chronological feed (S1); kept while the ranking is shown. */
  order: GalleryOrder
  items: GalleryItem[]
  status: FeedStatus
  /** Always false in the ranking view, which has no further pages. */
  hasMore: boolean
  /** Id of the photo whose last like change was rolled back; null when none. */
  likeFailedId: string | null
}

export interface GalleryFeedOptions {
  fetchPage: (cursor: string | null, order: GalleryOrder) => Promise<GalleryPage>
  fetchTop: () => Promise<GalleryItem[]>
  sendLike: (id: string, liked: boolean) => Promise<LikeResult>
  onChange: (state: GalleryFeedState) => void
}

type LikeFields = Pick<GalleryItem, 'likeCount' | 'likedByMe'>

/**
 * Gallery state for both views. The chronological feed is cursor-paginated and
 * refreshed after uploads; the ranking is loaded once per view switch. Likes and
 * hides apply to both lists, so a photo shown twice stays consistent.
 */
export class GalleryFeed {
  readonly #options: GalleryFeedOptions
  #view: GalleryView = 'recent'
  #order: GalleryOrder = 'newest'
  // Bumped when the order changes so a page requested under the old order is dropped on arrival.
  #generation = 0
  #items: GalleryItem[] = []
  #cursor: string | null = null
  #hasMore = true
  #recentStatus: FeedStatus = 'idle'
  #top: GalleryItem[] = []
  #topStatus: FeedStatus = 'idle'
  #likeFailedId: string | null = null
  readonly #likesInFlight = new Set<string>()

  constructor(options: GalleryFeedOptions) {
    this.#options = options
  }

  get state(): GalleryFeedState {
    const recent = this.#view === 'recent'
    return {
      view: this.#view,
      order: this.#order,
      items: [...(recent ? this.#items : this.#top)],
      status: recent ? this.#recentStatus : this.#topStatus,
      hasMore: recent && this.#hasMore,
      likeFailedId: this.#likeFailedId
    }
  }

  /**
   * Loads the next older page in the chronological view. In the ranking view it
   * only retries a failed ranking load; the ranking has no pages.
   */
  async loadMore(): Promise<void> {
    if (this.#view === 'top') {
      if (this.#topStatus === 'error') await this.#loadTop()
      return
    }
    if (this.#recentStatus === 'loading' || !this.#hasMore) return
    const generation = this.#generation
    const page = await this.#fetch(this.#cursor)
    if (page === null || generation !== this.#generation) return
    const known = new Set(this.#items.map((entry) => entry.id))
    this.#items.push(...page.items.filter((entry) => !known.has(entry.id)))
    this.#cursor = page.nextCursor
    this.#hasMore = page.nextCursor !== null
    this.#emit()
  }

  /** Switches between "Recientes" and "Más gustadas". The ranking is reloaded on every switch to it. */
  async setView(view: GalleryView): Promise<void> {
    if (view === this.#view) return
    this.#view = view
    this.#emit()
    if (view === 'top') await this.#loadTop()
  }

  /**
   * Switches the chronological feed between newest-first and oldest-first (S1, S6). Cursors belong
   * to one order, so the list starts again from the first page. The ranking view is unaffected.
   */
  async setOrder(order: GalleryOrder): Promise<void> {
    if (order === this.#order) return
    this.#order = order
    this.#generation += 1
    this.#items = []
    this.#cursor = null
    this.#hasMore = true
    this.#recentStatus = 'idle'
    this.#emit()
    if (this.#view === 'recent') await this.loadMore()
  }

  /** Drops one photo from both lists (e.g. after it was hidden). Does not refetch. */
  remove(id: string): void {
    const remaining = this.#items.filter((entry) => entry.id !== id)
    const topRemaining = this.#top.filter((entry) => entry.id !== id)
    if (remaining.length === this.#items.length && topRemaining.length === this.#top.length) return
    this.#items = remaining
    this.#top = topRemaining
    this.#emit()
  }

  /** Fetches the newest page and merges it on top, e.g. after an upload finishes. */
  async refresh(): Promise<void> {
    if (this.#recentStatus === 'loading') return
    if (this.#order === 'oldest') return this.#refreshOldest()
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

  /**
   * New uploads join the end of the oldest-first feed, behind a cursor the client cannot build.
   * Once the guest has read to the end, the pages are read again and replace the list. Before
   * that the new photos arrive on their own as the guest keeps scrolling.
   */
  async #refreshOldest(): Promise<void> {
    if (this.#hasMore) return
    const generation = this.#generation
    const items: GalleryItem[] = []
    let cursor: string | null = null
    do {
      const page: GalleryPage | null = await this.#fetch(cursor)
      if (page === null || generation !== this.#generation) return
      items.push(...page.items)
      cursor = page.nextCursor
    } while (cursor !== null)
    const seen = new Set<string>()
    this.#items = items.filter((entry) => !seen.has(entry.id) && seen.add(entry.id))
    this.#cursor = null
    this.#hasMore = false
    this.#emit()
  }

  /**
   * Toggles the guest's like on one photo. The change shows at once; the server
   * answer replaces the optimistic values, and a failure restores the previous
   * values. A photo with a request in flight ignores further toggles.
   */
  async toggleLike(id: string): Promise<void> {
    if (this.#likesInFlight.has(id)) return
    const current = this.#currentList().find((entry) => entry.id === id)
    if (current === undefined) return

    const previous: LikeFields = { likeCount: current.likeCount, likedByMe: current.likedByMe }
    const liked = !current.likedByMe
    this.#likesInFlight.add(id)
    this.#likeFailedId = null
    this.#patch(id, () => ({
      likedByMe: liked,
      likeCount: Math.max(0, previous.likeCount + (liked ? 1 : -1))
    }))
    this.#emit()

    try {
      const result = await this.#options.sendLike(id, liked)
      this.#patch(id, () => ({ likeCount: result.likeCount, likedByMe: result.likedByMe }))
    } catch (error) {
      this.#patch(id, () => previous)
      this.#likeFailedId = id
      if (error instanceof GuestSessionRequiredError) this.#setCurrentStatus('session_required')
    } finally {
      this.#likesInFlight.delete(id)
    }
    this.#emit()
  }

  async #loadTop(): Promise<void> {
    if (this.#topStatus === 'loading') return
    this.#topStatus = 'loading'
    this.#emit()
    try {
      this.#top = await this.#options.fetchTop()
      this.#topStatus = 'idle'
    } catch (error) {
      this.#topStatus = error instanceof GuestSessionRequiredError ? 'session_required' : 'error'
    }
    this.#emit()
  }

  async #fetch(cursor: string | null): Promise<GalleryPage | null> {
    this.#recentStatus = 'loading'
    this.#emit()
    const generation = this.#generation
    try {
      const page = await this.#options.fetchPage(cursor, this.#order)
      // A page for a previous order must not clear the status of the current one.
      if (generation === this.#generation) this.#recentStatus = 'idle'
      return page
    } catch (error) {
      if (generation !== this.#generation) return null
      this.#recentStatus = error instanceof GuestSessionRequiredError ? 'session_required' : 'error'
      this.#emit()
      return null
    }
  }

  #currentList(): GalleryItem[] {
    return this.#view === 'recent' ? this.#items : this.#top
  }

  #setCurrentStatus(status: FeedStatus): void {
    if (this.#view === 'recent') this.#recentStatus = status
    else this.#topStatus = status
  }

  /** Applies a like-field change to the photo in both lists. Does not emit. */
  #patch(id: string, change: (entry: GalleryItem) => LikeFields): void {
    const apply = (entry: GalleryItem): GalleryItem => (entry.id === id ? { ...entry, ...change(entry) } : entry)
    this.#items = this.#items.map(apply)
    this.#top = this.#top.map(apply)
  }

  #emit(): void {
    this.#options.onChange(this.state)
  }
}
