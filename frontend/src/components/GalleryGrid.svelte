<script lang="ts">
  import type { GalleryFeedState, GalleryView } from '../lib/gallery/feed'
  import { computeWindow } from '../lib/gallery/window'
  import { gridImage, isVideoItem, photoHash, shouldShowLikeBadge } from '../lib/gallery/view'
  import { copy, likeLabels } from '../lib/i18n/es'

  let {
    feed,
    onLoadMore,
    onOpen,
    onLike
  }: {
    feed: GalleryFeedState
    onLoadMore: () => void
    /** Opens the viewer for an item; plain link behaviour is kept for modified clicks. */
    onOpen: (id: string) => void
    onLike: (id: string) => void
  } = $props()

  function open(event: MouseEvent, id: string): void {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    onOpen(id)
  }

  // Id of the tile whose heart should pop: set by a tap that adds a like, cleared when the pop ends (S10).
  // A remounted tile (windowing) therefore never pops on its own.
  let popId = $state<string | null>(null)

  function like(id: string, likedByMe: boolean): void {
    if (!likedByMe) popId = id
    onLike(id)
  }

  let sentinel = $state<HTMLElement>()
  let gridEl = $state<HTMLUListElement>()

  // Mounted slice of feed.items plus spacer heights (px) that keep the grid's full height (S1, S2).
  let slice = $state({ start: 0, end: 0, topSpacer: 0, bottomSpacer: 0 })
  const visibleItems = $derived(feed.items.slice(slice.start, slice.end))
  let frame = 0
  let shownView: GalleryView | undefined

  /** Recomputes the window from the grid's layout. `top` resets it to the first rows (D4). */
  function measure(top: boolean): void {
    if (gridEl === undefined) return
    const style = getComputedStyle(gridEl)
    const tracks = (style.gridTemplateColumns ?? '').trim().split(/\s+/).filter(Boolean)
    const tileWidth = parseFloat(tracks[0] ?? '')
    const gap = parseFloat(style.rowGap ?? '') || 0
    // No layout (e.g. jsdom) gives rowHeight 0, which yields a bounded first window without spacers.
    const rowHeight = Number.isFinite(tileWidth) ? tileWidth + gap : 0
    const range = computeWindow({
      scrollOffset: top ? 0 : -gridEl.getBoundingClientRect().top,
      viewportHeight: window.innerHeight,
      rowHeight,
      columns: tracks.length,
      total: feed.items.length
    })
    // Each spacer sits one row gap short: the gap that follows it completes the pitch.
    slice = {
      start: range.start,
      end: range.end,
      topSpacer: range.topSpacer > 0 ? range.topSpacer - gap : 0,
      bottomSpacer: range.bottomSpacer > 0 ? range.bottomSpacer - gap : 0
    }
  }

  function scheduleMeasure(): void {
    if (frame !== 0) return
    if (typeof requestAnimationFrame !== 'function') return measure(false)
    frame = requestAnimationFrame(() => {
      frame = 0
      measure(false)
    })
  }

  // Window scroll and resize move the viewport over the rows; passive, rAF-throttled.
  $effect(() => {
    window.addEventListener('scroll', scheduleMeasure, { passive: true })
    window.addEventListener('resize', scheduleMeasure, { passive: true })
    return () => {
      window.removeEventListener('scroll', scheduleMeasure)
      window.removeEventListener('resize', scheduleMeasure)
      if (frame !== 0 && typeof cancelAnimationFrame === 'function') cancelAnimationFrame(frame)
      frame = 0
    }
  })

  // Re-window when the list changes; a view switch starts again at the top (D4).
  // Opening the photo dialog does not change the list or the scroll, so the window stays put (D3).
  $effect(() => {
    const view = feed.view
    const total = feed.items.length
    const switched = shownView !== undefined && shownView !== view
    shownView = view
    void total
    measure(switched)
  })

  // Load older photos as the guest scrolls near the end of the grid.
  $effect(() => {
    if (sentinel === undefined || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        // The ranking has no pages, so scrolling past it must not reload it.
        if (feed.hasMore && entries.some((entry) => entry.isIntersecting)) onLoadMore()
      },
      { rootMargin: '600px 0px' }
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  })
</script>

<section id="gallery" class="gallery" aria-labelledby="gallery-heading">
  <h2 id="gallery-heading">{copy.gallery.heading}</h2>

  {#if feed.likeFailedId !== null}
    <p class="notice" role="alert">{copy.likes.failed}</p>
  {/if}

  {#if feed.items.length === 0 && feed.status === 'idle' && !feed.hasMore}
    <p class="notice">{feed.view === 'top' ? copy.ranking.empty : copy.gallery.empty}</p>
  {/if}

  <ul class="grid" bind:this={gridEl}>
    {#if slice.topSpacer > 0}
      <li class="spacer" aria-hidden="true" style:height="{slice.topSpacer}px"></li>
    {/if}
    {#each visibleItems as item (item.id)}
      {@const image = gridImage(item)}
      <li class="cell">
        <a class="tile" href={photoHash(item.id)} onclick={(event) => open(event, item.id)}>
          {#if image !== null}
            <img src={image} alt={copy.gallery.photoAlt} loading="lazy" decoding="async" />
          {:else}
            <span class="placeholder">{copy.gallery.noPreview}</span>
          {/if}
          {#if isVideoItem(item)}
            <span class="play-badge" role="img" aria-label={copy.gallery.videoLabel}>▶</span>
          {/if}
        </a>
        {#if shouldShowLikeBadge(item)}
          <!-- The button is a 44 px hit area around the compact pill; the name carries the count (S10). -->
          <button
            class="like"
            class:liked={item.likedByMe}
            type="button"
            aria-label={likeLabels.button(item.likedByMe, item.likeCount)}
            title={item.likedByMe ? copy.likes.unlike : copy.likes.like}
            onclick={() => like(item.id, item.likedByMe)}
          >
            <span class="pill">
              <span class="heart" class:pop={popId === item.id} aria-hidden="true" onanimationend={() => (popId = null)}>
                {item.likedByMe ? '♥' : '♡'}
              </span>
              <span aria-hidden="true">{item.likeCount}</span>
            </span>
          </button>
        {/if}
      </li>
    {/each}
    {#if slice.bottomSpacer > 0}
      <li class="spacer" aria-hidden="true" style:height="{slice.bottomSpacer}px"></li>
    {/if}
  </ul>

  <div bind:this={sentinel} class="gallery-footer">
    {#if feed.status === 'loading'}
      <p class="notice" role="status">{copy.gallery.loading}</p>
    {:else if feed.status === 'error'}
      <p class="notice" role="alert">{copy.gallery.error}</p>
      <button class="button" type="button" onclick={onLoadMore}>{copy.gallery.retry}</button>
    {:else if feed.hasMore}
      <button class="button" type="button" onclick={onLoadMore}>{copy.gallery.loadMore}</button>
    {:else if feed.view === 'recent' && feed.items.length > 0}
      <p class="notice">{copy.gallery.end}</p>
    {/if}
  </div>
</section>

<style>
  .gallery h2 {
    text-align: center;
  }

  .grid {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(30vw, 160px), 1fr));
    gap: 0.4rem;
  }

  .cell {
    position: relative;
  }

  .spacer {
    grid-column: 1 / -1;
  }

  /* 44 px square hit area; only the compact pill inside is painted. */
  .like {
    position: absolute;
    left: 0;
    bottom: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    box-sizing: border-box;
    min-width: 2.75rem;
    min-height: 2.75rem;
    padding: 0;
    border: none;
    background: none;
    cursor: pointer;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.15rem 0.5rem;
    border-radius: 999px;
    background: rgb(29 26 23 / 0.6);
    color: #fff;
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .like.liked .pill {
    color: var(--danger-soft);
  }

  .like:focus-visible {
    outline: 3px solid var(--sage);
    outline-offset: 2px;
  }

  .tile {
    position: relative;
    display: block;
    aspect-ratio: 1;
    overflow: hidden;
    border-radius: 0.4rem;
    background: var(--line);
  }

  .tile:focus-visible {
    outline: 3px solid var(--sage);
    outline-offset: 2px;
  }

  .tile img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .play-badge {
    position: absolute;
    right: 0.4rem;
    bottom: 0.4rem;
    display: grid;
    place-items: center;
    width: 1.8rem;
    height: 1.8rem;
    border-radius: 50%;
    background: rgb(29 26 23 / 0.7);
    color: #fff;
    font-size: 0.8rem;
    pointer-events: none;
  }

  .placeholder {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 0.5rem;
    text-align: center;
    font-size: 0.8rem;
    color: var(--muted);
  }

  .gallery-footer {
    display: grid;
    justify-items: center;
    gap: 0.5rem;
    padding: 1.5rem 0;
  }

</style>
