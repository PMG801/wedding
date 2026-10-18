<script lang="ts">
  import { onMount } from 'svelte'
  import { AdminSessionRequiredError } from '../lib/api/admin'
  import { fetchHiddenPage, setMediaVisibility, type HiddenItem } from '../lib/api/moderation'
  import { adminCopy } from '../lib/i18n/admin'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  let items = $state<HiddenItem[]>([])
  let nextCursor = $state<string | null>(null)
  let loaded = $state(false)
  let busy = $state(false)
  let message = $state('')

  onMount(() => {
    void loadPage(null)
  })

  async function loadPage(cursor: string | null): Promise<void> {
    if (busy) return
    busy = true
    message = ''
    try {
      const page = await fetchHiddenPage({ cursor })
      items = cursor === null ? page.items : [...items, ...page.items]
      nextCursor = page.nextCursor
      loaded = true
    } catch (error) {
      if (error instanceof AdminSessionRequiredError) onSessionLost()
      else message = adminCopy.moderation.loadFailed
    } finally {
      busy = false
    }
  }

  async function show(item: HiddenItem): Promise<void> {
    if (busy) return
    busy = true
    message = ''
    try {
      await setMediaVisibility(item.id, 'visible')
      items = items.filter((entry) => entry.id !== item.id)
    } catch (error) {
      if (error instanceof AdminSessionRequiredError) onSessionLost()
      else message = adminCopy.moderation.failed
    } finally {
      busy = false
    }
  }
</script>

<section class="hidden-media" aria-labelledby="hidden-heading">
  <h3 id="hidden-heading">{adminCopy.moderation.heading}</h3>

  {#if loaded && items.length === 0}
    <p class="notice">{adminCopy.moderation.empty}</p>
  {/if}

  <ul class="grid">
    {#each items as item (item.id)}
      <li>
        {#if item.thumbUrl !== null}
          <img src={item.thumbUrl} alt={adminCopy.moderation.thumbAlt} loading="lazy" decoding="async" />
        {:else}
          <span class="placeholder">{adminCopy.moderation.noPreview}</span>
        {/if}
        <button class="button small" type="button" onclick={() => show(item)} disabled={busy}>
          {adminCopy.moderation.show}
        </button>
      </li>
    {/each}
  </ul>

  {#if nextCursor !== null}
    <button class="button" type="button" onclick={() => loadPage(nextCursor)} disabled={busy}>
      {adminCopy.moderation.loadMore}
    </button>
  {/if}

  {#if message !== ''}
    <p class="admin-error" role="alert">{message}</p>
  {/if}
</section>

<style>
  .hidden-media h3 {
    margin: 0.5rem 0 0;
    font-size: 1.25rem;
  }

  .grid {
    margin: 0;
    padding: 0;
    list-style: none;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(7rem, 1fr));
    gap: 0.5rem;
  }

  .grid li {
    display: grid;
    gap: 0.35rem;
  }

  .grid img,
  .placeholder {
    width: 100%;
    aspect-ratio: 1;
    object-fit: cover;
    border-radius: 0.5rem;
    background: var(--line);
  }

  .placeholder {
    display: grid;
    place-items: center;
    color: var(--muted);
    font-size: 0.8rem;
    text-align: center;
  }

  .notice {
    margin: 0;
    color: var(--muted);
  }

  .admin-error {
    margin: 0;
    color: var(--danger);
  }
</style>
