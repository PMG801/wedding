<script lang="ts">
  import { onMount } from 'svelte'
  import MediaLightbox from './MediaLightbox.svelte'
  import { GuestSessionRequiredError } from '../lib/api/media'
  import { fetchPinned, type PinnedImage, type PinnedItem } from '../lib/api/pinned'
  import { closeViewer, openViewer } from '../lib/gallery/history'
  import { copy } from '../lib/i18n/es'
  import { readCollapsed, writeCollapsed } from '../lib/pinned/collapse'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  let items = $state<PinnedItem[]>([])
  let collapsed = $state(readCollapsed())
  let opened = $state<PinnedImage | null>(null)

  onMount(() => {
    void load()
    // The phone back button closes an open pinned image (S4).
    const onPopState = () => (opened = null)
    addEventListener('popstate', onPopState)
    return () => removeEventListener('popstate', onPopState)
  })

  function openImage(image: PinnedImage): void {
    openViewer(history, location.href)
    opened = image
  }

  function closeImage(): void {
    if (opened === null) return
    if (closeViewer(history, location.href) === 'replaced') opened = null
  }

  /** A failed read hides the section quietly so the gallery below is not affected (S4). */
  async function load(): Promise<void> {
    try {
      items = await fetchPinned()
    } catch (error) {
      items = []
      if (error instanceof GuestSessionRequiredError) onSessionLost()
    }
  }

  function toggle(): void {
    collapsed = !collapsed
    writeCollapsed(collapsed)
  }
</script>

{#if items.length > 0}
  <section class="pinned" aria-labelledby="pinned-heading">
    <div class="pinned-head">
      <h2 id="pinned-heading">{copy.pinned.heading}</h2>
      <button
        class="button small"
        type="button"
        aria-expanded={!collapsed}
        aria-controls="pinned-body"
        onclick={toggle}
      >
        {collapsed ? copy.pinned.expand : copy.pinned.collapse}
      </button>
    </div>

    <div id="pinned-body" hidden={collapsed}>
      <ul class="pinned-list">
        {#each items as item (item.id)}
          <li class="pinned-item">
            {#if item.image !== null}
              <button
                class="pinned-open"
                type="button"
                aria-label={copy.pinned.openImage}
                onclick={() => item.image !== null && openImage(item.image)}
              >
                <img src={item.image.thumbUrl} alt={copy.pinned.imageAlt} loading="lazy" decoding="async" />
              </button>
            {/if}
            <div class="pinned-text">
              <h3>{item.title}</h3>
              {#if item.body !== ''}
                <p>{item.body}</p>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    </div>
  </section>
{/if}

{#if opened !== null}
  <MediaLightbox label={copy.pinned.imageAlt} onClose={closeImage}>
    <img src={opened.detailUrl ?? opened.thumbUrl} alt={copy.pinned.imageAlt} decoding="async" />
  </MediaLightbox>
{/if}

<style>
  .pinned {
    display: grid;
    gap: 0.75rem;
    margin: 0 0 1.25rem;
    padding: 1rem;
    border: 1px solid var(--line);
    border-radius: 0.8rem;
    background: var(--paper);
  }

  .pinned-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .pinned h2 {
    margin: 0;
    font-size: 1.5rem;
  }

  .pinned-list {
    display: grid;
    gap: 0.75rem;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .pinned-item {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.75rem;
    align-items: start;
  }

  .pinned-open {
    padding: 0;
    border: none;
    background: none;
    cursor: zoom-in;
    line-height: 0;
  }

  .pinned-item img {
    width: 5rem;
    height: 5rem;
    object-fit: cover;
    border-radius: 0.5rem;
    background: var(--line);
  }

  .pinned-text h3 {
    margin: 0;
    font-size: 1.1rem;
  }

  .pinned-text p {
    margin: 0.25rem 0 0;
    color: var(--ink);
    white-space: pre-line;
    overflow-wrap: anywhere;
  }
</style>
