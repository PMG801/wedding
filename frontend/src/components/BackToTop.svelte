<script lang="ts">
  import { onMount } from 'svelte'
  import { shouldShowBackToTop } from '../lib/gallery/backToTop'
  import { copy } from '../lib/i18n/es'

  /** Floating button that returns to the very top of the page (S14); `offset` keeps it above the dock. */
  let { offset }: { offset: number } = $props()

  let visible = $state(false)

  onMount(() => {
    const update = () => (visible = shouldShowBackToTop(scrollY, innerHeight))
    update()
    // Passive listener; the state only changes when the threshold is crossed, so scrolling stays cheap.
    addEventListener('scroll', update, { passive: true })
    return () => removeEventListener('scroll', update)
  })

  function toTop() {
    const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' })
  }
</script>

<button
  class="top"
  class:visible
  type="button"
  aria-label={copy.gallery.backToTop}
  tabindex={visible ? 0 : -1}
  aria-hidden={!visible}
  style:bottom="calc({offset}px + 0.75rem)"
  onclick={toTop}
>
  <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
    <path d="M12 19V5M5 12l7-7 7 7" />
  </svg>
</button>

<style>
  .top {
    position: fixed;
    right: max(1rem, calc((100vw - 960px) / 2 + 1rem));
    z-index: 19;
    width: 44px;
    height: 44px;
    display: grid;
    place-items: center;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: 50%;
    background: var(--paper);
    color: var(--sage-dark);
    box-shadow: 0 2px 10px rgb(47 42 38 / 0.15);
    cursor: pointer;
    opacity: 0;
    transform: translateY(8px);
    pointer-events: none;
    transition:
      opacity 0.2s ease,
      transform 0.2s ease;
  }

  .top.visible {
    opacity: 1;
    transform: none;
    pointer-events: auto;
  }

  svg {
    width: 22px;
    height: 22px;
    fill: none;
    stroke: currentColor;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  @media (prefers-reduced-motion: reduce) {
    .top {
      transition: none;
    }
  }
</style>
