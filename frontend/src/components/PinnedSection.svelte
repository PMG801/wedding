<script lang="ts">
  import { onMount } from 'svelte'
  import MediaLightbox from './MediaLightbox.svelte'
  import { GuestSessionRequiredError } from '../lib/api/media'
  import { fetchPinned, type PinnedImage, type PinnedItem } from '../lib/api/pinned'
  import { closeViewer, openViewer } from '../lib/gallery/history'
  import { copy } from '../lib/i18n/es'
  import { bodyOverflows } from '../lib/pinned/clamp'
  import { readCollapsed, writeCollapsed } from '../lib/pinned/collapse'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  let items = $state<PinnedItem[]>([])
  let collapsed = $state(readCollapsed())
  let opened = $state<PinnedImage | null>(null)
  // Per card, keyed by item id: whether the clamped body hides text, and whether the guest expanded it (S9).
  let overflowing = $state<Record<string, boolean>>({})
  let expanded = $state<Record<string, boolean>>({})

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

  function toggleBody(id: string): void {
    expanded[id] = expanded[id] !== true
  }

  /**
   * Measures whether a clamped body hides text. A collapsed section has no layout, so the
   * observer re-measures once the section is shown again. An expanded body is not clamped,
   * so its last answer is kept and the toggle stays available to fold it back.
   */
  function watchOverflow(node: HTMLElement, id: string) {
    const measure = () => {
      if (expanded[id] === true) return
      overflowing[id] = bodyOverflows(node.scrollHeight, node.clientHeight)
    }
    measure()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(node)
    return { destroy: () => observer?.disconnect() }
  }
</script>

{#if items.length > 0}
  <section class="pinned" aria-labelledby="pinned-heading">
    <header class="pinned-head">
      <svg class="pin" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
        <path d="M9 4h6l-1 6 3 3H7l3-3-1-6z M12 13v7" />
      </svg>
      <h2 id="pinned-heading">{copy.pinned.heading}</h2>
      <span class="count">{items.length}</span>
      <button
        class="toggle"
        type="button"
        aria-expanded={!collapsed}
        aria-controls="pinned-body"
        aria-label={collapsed ? copy.pinned.show : copy.pinned.hide}
        onclick={toggle}
      >
        <svg class="chevron" class:open={!collapsed} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
    </header>

    <div id="pinned-body" hidden={collapsed}>
      <!-- A horizontally scrollable region must be focusable so the keyboard can scroll it (WCAG 2.1.1). -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <ul class="pinned-list" tabindex="0" aria-label={copy.pinned.heading}>
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
                <p
                  id="pinned-text-{item.id}"
                  class="body"
                  class:clamped={expanded[item.id] !== true}
                  use:watchOverflow={item.id}>{item.body}</p>
                {#if overflowing[item.id] === true}
                  <button
                    class="more"
                    type="button"
                    aria-expanded={expanded[item.id] === true}
                    aria-controls="pinned-text-{item.id}"
                    aria-label={expanded[item.id] === true ? copy.pinned.readLess : copy.pinned.readMore}
                    onclick={() => toggleBody(item.id)}
                  >
                    <svg class="chevron" class:open={expanded[item.id] === true} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
                      <path d="M6 9l6 6 6-6" />
                    </svg>
                  </button>
                {/if}
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
    margin: 0 0 1.25rem;
  }

  .pinned-head {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    min-height: 2.75rem;
  }

  .pinned h2 {
    margin: 0;
    font-size: 1.4rem;
  }

  .pin {
    width: 1.1rem;
    height: 1.1rem;
    flex-shrink: 0;
    fill: none;
    stroke: var(--sage-dark);
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .count {
    color: var(--muted);
    font-size: 0.9rem;
  }

  .toggle,
  .more {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 2.75rem;
    height: 2.75rem;
    padding: 0;
    border: 0;
    border-radius: 999px;
    background: none;
    color: var(--sage-dark);
    cursor: pointer;
  }

  .toggle {
    margin-left: auto;
  }

  .more {
    justify-self: center;
    margin: -0.4rem 0 -0.5rem;
  }

  .toggle:focus-visible,
  .more:focus-visible,
  .pinned-open:focus-visible {
    outline: 2px solid var(--sage);
    outline-offset: 2px;
  }

  .chevron {
    width: 1.25rem;
    height: 1.25rem;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
    transition: transform 180ms ease;
  }

  .chevron.open {
    transform: rotate(180deg);
  }

  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }

  /* One card per row, the next card peeks in; the row scrolls horizontally with snap (S9). */
  .pinned-list {
    display: flex;
    gap: 0.75rem;
    margin: 0.5rem 0 0;
    padding: 0.2rem 0 0.4rem;
    list-style: none;
    align-items: flex-start;
    overflow-x: auto;
    overscroll-behavior-x: contain;
    scroll-snap-type: x mandatory;
    scrollbar-width: none;
  }

  .pinned-list::-webkit-scrollbar {
    display: none;
  }

  .pinned-item {
    flex: 0 0 82%;
    display: flex;
    flex-direction: column;
    min-width: 0;
    overflow: hidden;
    scroll-snap-align: start;
    border-top: 3px solid var(--sage);
    border-radius: 0.9rem;
    background: var(--paper);
    box-shadow: 0 1px 3px rgb(47 42 38 / 0.08);
  }

  .pinned-item:only-child {
    flex-basis: 100%;
  }

  .pinned-open {
    display: block;
    width: 100%;
    padding: 0;
    border: 0;
    background: none;
    cursor: zoom-in;
    line-height: 0;
  }

  .pinned-item img {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    object-fit: cover;
    background: var(--line);
  }

  .pinned-text {
    display: grid;
    gap: 0.3rem;
    padding: 0.7rem 0.9rem 0.85rem;
  }

  .pinned-text h3 {
    margin: 0;
    font-family: var(--serif);
    font-size: 1.25rem;
    font-weight: 500;
    line-height: 1.2;
    overflow-wrap: anywhere;
  }

  .body {
    margin: 0;
    color: var(--ink);
    font-size: 0.95rem;
    line-height: 1.5;
    white-space: pre-line;
    overflow-wrap: anywhere;
  }

  /* Four lines at line-height 1.5: longer bodies get the read-more toggle (S9). */
  .body.clamped {
    max-height: 6em;
    overflow: hidden;
  }
</style>
