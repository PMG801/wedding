<script lang="ts">
  import type { CarouselPeriod } from '../lib/api/carousels'
  import { periodLabel } from '../lib/carousel/label'
  import { copy } from '../lib/i18n/es'

  let { periods, onOpen }: { periods: CarouselPeriod[]; onOpen: (id: number) => void } = $props()

  // A period without a readable time or without media has no bubble.
  const bubbles = $derived(
    periods.flatMap((period) => {
      const label = periodLabel(period.start, period.end)
      if (label === null || period.items.length === 0) return []
      return [{ id: period.id, label, thumb: period.items[0].thumbUrl }]
    })
  )
</script>

{#if bubbles.length > 0}
  <section class="strip" aria-label={copy.carousel.heading}>
    <ul>
      {#each bubbles as bubble (bubble.id)}
        <li>
          <button type="button" class="bubble" onclick={() => onOpen(bubble.id)} aria-label={copy.carousel.bubble(bubble.label)}>
            {#if bubble.thumb !== null}
              <img src={bubble.thumb} alt="" loading="lazy" decoding="async" />
            {/if}
          </button>
          <span class="label" aria-hidden="true">{bubble.label}</span>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .strip {
    padding: 0.75rem 0 0.25rem;
  }

  ul {
    display: flex;
    gap: 0.9rem;
    margin: 0;
    padding: 0 1rem;
    overflow-x: auto;
    list-style: none;
    scrollbar-width: none;
  }

  li {
    display: flex;
    flex: 0 0 auto;
    flex-direction: column;
    align-items: center;
    gap: 0.25rem;
  }

  .bubble {
    width: 4.25rem;
    height: 4.25rem;
    padding: 2px;
    border: none;
    border-radius: 50%;
    background: var(--sage);
    cursor: pointer;
    overflow: hidden;
  }

  .bubble:focus-visible {
    outline: 3px solid var(--sage-dark);
    outline-offset: 2px;
  }

  .bubble img {
    display: block;
    width: 100%;
    height: 100%;
    border: 2px solid var(--ivory);
    border-radius: 50%;
    object-fit: cover;
  }

  .label {
    color: var(--muted);
    font-size: var(--text-xs);
    white-space: nowrap;
  }
</style>
