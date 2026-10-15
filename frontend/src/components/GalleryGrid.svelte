<script lang="ts">
  import type { GalleryFeedState } from '../lib/gallery/feed'
  import { gridImage, photoHash } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  let { feed, onLoadMore }: { feed: GalleryFeedState; onLoadMore: () => void } = $props()

  let sentinel = $state<HTMLElement>()

  // Load older photos as the guest scrolls near the end of the grid.
  $effect(() => {
    if (sentinel === undefined || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) onLoadMore()
      },
      { rootMargin: '600px 0px' }
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  })
</script>

<section class="gallery" aria-labelledby="gallery-heading">
  <h2 id="gallery-heading">{copy.gallery.heading}</h2>

  {#if feed.items.length === 0 && feed.status === 'idle' && !feed.hasMore}
    <p class="notice">{copy.gallery.empty}</p>
  {/if}

  <ul class="grid">
    {#each feed.items as item (item.id)}
      {@const image = gridImage(item)}
      <li>
        <a class="tile" href={photoHash(item.id)}>
          {#if image !== null}
            <img src={image} alt={copy.gallery.photoAlt} loading="lazy" decoding="async" />
          {:else}
            <span class="placeholder">{copy.gallery.noPreview}</span>
          {/if}
        </a>
      </li>
    {/each}
  </ul>

  <div bind:this={sentinel} class="gallery-footer">
    {#if feed.status === 'loading'}
      <p class="notice" role="status">{copy.gallery.loading}</p>
    {:else if feed.status === 'error'}
      <p class="notice" role="alert">{copy.gallery.error}</p>
      <button class="button" type="button" onclick={onLoadMore}>{copy.gallery.retry}</button>
    {:else if feed.hasMore}
      <button class="button" type="button" onclick={onLoadMore}>{copy.gallery.loadMore}</button>
    {:else if feed.items.length > 0}
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

  .tile {
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
