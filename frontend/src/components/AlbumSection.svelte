<script lang="ts">
  import StoryViewer from './StoryViewer.svelte'
  import { fetchPeriodPage, PeriodNotFoundError } from '../lib/api/carousels'
  import { GuestSessionRequiredError, type GalleryItem } from '../lib/api/media'
  import { gridImage, isVideoItem } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  /**
   * The full album of one period (S9), paged with the keyset cursor. Loaded as a
   * lazy chunk from the home page (#/album/<id>); tiles open the story viewer.
   */
  let { periodId, label, onBack, onSessionLost }: {
    periodId: number
    /** Time range of the period for the heading; null when the strip does not know it. */
    label: string | null
    onBack: () => void
    onSessionLost?: () => void
  } = $props()

  type Status = 'loading' | 'ready' | 'error' | 'missing'
  let status = $state<Status>('loading')
  let items = $state<GalleryItem[]>([])
  let cursor = $state<string | null>(null)
  let loadingMore = $state(false)
  let moreFailed = $state(false)
  let viewing = $state<number | null>(null)
  // Bumped on every period change so late responses of an old album are dropped.
  let generation = 0

  $effect(() => {
    const id = periodId
    const current = ++generation
    status = 'loading'
    items = []
    cursor = null
    moreFailed = false
    viewing = null
    fetchPeriodPage({ id }).then(
      (page) => {
        if (current !== generation) return
        items = page.items
        cursor = page.nextCursor
        status = 'ready'
      },
      (error: unknown) => {
        if (current === generation) fail(error)
      }
    )
  })

  function fail(error: unknown) {
    if (error instanceof GuestSessionRequiredError) return onSessionLost?.()
    status = error instanceof PeriodNotFoundError ? 'missing' : 'error'
  }

  async function loadMore() {
    if (cursor === null || loadingMore) return
    const current = generation
    const id = periodId
    loadingMore = true
    moreFailed = false
    try {
      const page = await fetchPeriodPage({ id, cursor })
      if (current !== generation) return
      items = [...items, ...page.items]
      cursor = page.nextCursor
    } catch (error) {
      if (current === generation) {
        if (error instanceof GuestSessionRequiredError) onSessionLost?.()
        else moreFailed = true
      }
    } finally {
      loadingMore = false
    }
  }
</script>

<section class="album" aria-labelledby="album-title">
  <header class="bar">
    <button class="button" type="button" onclick={onBack}>‹ {copy.carousel.back}</button>
    <h2 id="album-title">{label ?? copy.carousel.heading}</h2>
  </header>

  {#if status === 'loading'}
    <p class="notice" role="status">{copy.gallery.loading}</p>
  {:else if status === 'missing'}
    <p class="notice" role="alert">{copy.carousel.albumNotFound}</p>
  {:else if status === 'error'}
    <p class="notice" role="alert">{copy.carousel.albumError}</p>
  {:else if items.length === 0}
    <p class="notice">{copy.carousel.albumEmpty}</p>
  {:else}
    <ul class="grid">
      {#each items as item, index (item.id)}
        <li>
          <button class="tile" type="button" onclick={() => (viewing = index)} aria-label={isVideoItem(item) ? copy.gallery.videoLabel : copy.gallery.photoAlt}>
            {#if gridImage(item) !== null}
              <img src={gridImage(item)} alt="" loading="lazy" decoding="async" />
            {:else}
              <span class="placeholder">{isVideoItem(item) ? copy.gallery.videoLabel : copy.gallery.noPreview}</span>
            {/if}
          </button>
        </li>
      {/each}
    </ul>
    {#if cursor !== null}
      <div class="more">
        <button class="button" type="button" onclick={loadMore} disabled={loadingMore}>{copy.gallery.loadMore}</button>
        {#if moreFailed}
          <p class="notice" role="alert">{copy.carousel.albumError}</p>
        {/if}
      </div>
    {/if}
  {/if}
</section>

{#if viewing !== null}
  <StoryViewer
    items={items}
    label={label ?? copy.carousel.heading}
    start={viewing}
    onClose={() => (viewing = null)}
    onEnd={() => void loadMore()}
  />
{/if}

<style>
  .album {
    box-sizing: border-box;
    height: 100%;
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--ivory);
    padding: 0 0.75rem 1.5rem;
  }

  .bar {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    min-height: 3.25rem;
    margin: 0 -0.75rem;
    padding: 0 0.75rem;
    background: var(--ivory);
  }

  h2 {
    margin: 0;
    font-size: 1.35rem;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 4px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .tile {
    display: block;
    width: 100%;
    aspect-ratio: 1;
    padding: 0;
    border: none;
    background: var(--line);
    cursor: pointer;
    overflow: hidden;
  }

  .tile img {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .placeholder {
    color: var(--muted);
    font-size: 0.8rem;
  }

  .more {
    display: grid;
    justify-items: center;
    gap: 0.5rem;
    margin-top: 1rem;
  }
</style>
