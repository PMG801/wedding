<script lang="ts">
  import MediaLightbox from './MediaLightbox.svelte'
  import type { GalleryItem } from '../lib/api/media'
  import { clampIndex, storyStep, storyTap, type StoryStep } from '../lib/carousel/story'
  import { detailImage, isVideoItem, nextPreload, videoPoster } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  /**
   * Story viewer over the items of one period (S8). Tap halves, swipe and arrow
   * keys move; the last item reports `onEnd`, which closes the story by default.
   * Reuses the gallery viewer shell, so the media and reduced-motion rules match.
   */
  let {
    items,
    label,
    onClose,
    onAlbum,
    onEnd,
    start = 0
  }: {
    items: GalleryItem[]
    label: string
    onClose: () => void
    /** Present for the strip: opens the period's full album. */
    onAlbum?: () => void
    /** Called past the last item; the strip closes, the album loads its next page. */
    onEnd?: () => void
    start?: number
  } = $props()

  // Each open mounts a new viewer, so the start index is read once on purpose.
  // svelte-ignore state_referenced_locally
  let index = $state(clampIndex(start, items.length))
  let pressed: { x: number; y: number } | null = null

  const position = $derived(clampIndex(index, items.length))
  const item = $derived(items[position] as GalleryItem | undefined)

  $effect(() => {
    // Fetch only the next image ahead, as the gallery viewer does (S11).
    const url = nextPreload(items, position)
    if (url === null) return
    const preloaded = new Image()
    preloaded.decoding = 'async'
    preloaded.src = url
  })

  function step(direction: StoryStep) {
    const next = storyStep(position, items.length, direction)
    if (next === 'end') return (onEnd ?? onClose)()
    index = next
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowLeft') step('previous')
    if (event.key === 'ArrowRight') step('next')
  }

  function onPointerDown(event: PointerEvent) {
    pressed = { x: event.clientX, y: event.clientY }
  }

  function onPointerUp(event: PointerEvent) {
    const start = pressed
    pressed = null
    // A video owns its taps (play, seek); only the picture and the bare frame turn pages.
    if (start === null || event.target instanceof HTMLVideoElement) return
    const frame = event.currentTarget as HTMLElement
    const rect = frame.getBoundingClientRect()
    const travel = Math.hypot(event.clientX - start.x, event.clientY - start.y)
    const direction = storyTap(event.clientX - rect.left, rect.width, travel)
    if (direction !== null) step(direction)
  }
</script>

<MediaLightbox
  {label}
  {onClose}
  onSwipe={step}
  {onKeydown}
  onFramePointerDown={onPointerDown}
  onFramePointerUp={onPointerUp}
>
  {#snippet topStart()}
    {#if onAlbum !== undefined}
      <button class="button small ghost" type="button" onclick={onAlbum}>{copy.carousel.viewAlbum}</button>
    {/if}
  {/snippet}

  {#if item !== undefined}
    {#if isVideoItem(item)}
      {#key item.id}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={item.originalUrl} poster={videoPoster(item)} controls playsinline preload="metadata"></video>
      {/key}
    {:else if detailImage(item) !== null}
      {#key item.id}
        <img src={detailImage(item)} alt={copy.gallery.photoAlt} decoding="async" />
      {/key}
    {:else}
      <p class="no-preview">{copy.detail.noPreview}</p>
    {/if}
  {/if}

  {#snippet controls()}
    <button class="button round ghost" type="button" onclick={() => step('previous')} disabled={position === 0} aria-label={copy.detail.previous}>‹</button>
    <p class="counter" aria-live="polite">{copy.carousel.counter(position + 1, items.length)}</p>
    <button class="button round ghost" type="button" onclick={() => step('next')} aria-label={copy.detail.next}>›</button>
  {/snippet}
</MediaLightbox>

<style>
  .no-preview {
    box-sizing: border-box;
    height: 100%;
    display: grid;
    place-items: center;
    max-width: 28rem;
    margin: 0 auto;
    padding: 1rem;
    text-align: center;
  }

  .counter {
    margin: 0;
    color: var(--paper);
    font-size: 0.9rem;
    white-space: nowrap;
  }
</style>
