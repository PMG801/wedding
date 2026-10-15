<script lang="ts">
  import type { GalleryItem } from '../lib/api/media'
  import { detailImage, downloadFileName, downloadUrl } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  let {
    items,
    index,
    hasMore,
    onNavigate,
    onClose
  }: {
    items: GalleryItem[]
    index: number
    hasMore: boolean
    onNavigate: (index: number) => void
    onClose: () => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let touchStartX: number | null = null

  const item = $derived(items[index])
  const image = $derived(detailImage(item))
  const hasPrevious = $derived(index > 0)
  const hasNext = $derived(index < items.length - 1 || hasMore)

  $effect(() => {
    if (dialog !== undefined && !dialog.open) dialog.showModal()
  })

  function previous() {
    if (hasPrevious) onNavigate(index - 1)
  }

  function next() {
    if (index < items.length - 1) onNavigate(index + 1)
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'ArrowLeft') previous()
    if (event.key === 'ArrowRight') next()
  }

  function onTouchEnd(event: TouchEvent) {
    if (touchStartX === null) return
    const deltaX = event.changedTouches[0].clientX - touchStartX
    touchStartX = null
    if (deltaX > 50) previous()
    if (deltaX < -50) next()
  }
</script>

<dialog
  bind:this={dialog}
  class="photo-dialog"
  aria-label={copy.gallery.photoAlt}
  onclose={onClose}
  onkeydown={onKeydown}
  ontouchstart={(event) => (touchStartX = event.touches[0].clientX)}
  ontouchend={onTouchEnd}
>
  <div class="frame">
    {#if image !== null}
      {#key image}
        <img src={image} alt={copy.gallery.photoAlt} decoding="async" />
      {/key}
    {:else}
      <p class="no-preview">{copy.detail.noPreview}</p>
    {/if}
  </div>

  <div class="controls">
    <button class="button round" type="button" onclick={previous} disabled={!hasPrevious} aria-label={copy.detail.previous}>‹</button>
    <a class="button primary" href={downloadUrl(item)} download={downloadFileName(item)}>{copy.detail.download}</a>
    <button class="button round" type="button" onclick={next} disabled={!hasNext} aria-label={copy.detail.next}>›</button>
  </div>

  <form method="dialog" class="close">
    <button class="button round" type="submit" aria-label={copy.detail.close}>✕</button>
  </form>
</dialog>

<style>
  .photo-dialog {
    width: 100vw;
    max-width: 100vw;
    height: 100dvh;
    max-height: 100dvh;
    margin: 0;
    padding: 0;
    border: none;
    background: #1d1a17;
    color: var(--paper);
    display: grid;
    grid-template-rows: 1fr auto;
  }

  .photo-dialog::backdrop {
    background: rgb(20 17 15 / 0.9);
  }

  .frame {
    min-height: 0;
    display: grid;
    place-items: center;
    padding: 3.5rem 0.5rem 0.5rem;
  }

  .frame img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    touch-action: pan-y pinch-zoom;
  }

  .no-preview {
    max-width: 28rem;
    padding: 1rem;
    text-align: center;
  }

  .controls {
    display: flex;
    justify-content: center;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem calc(0.75rem + env(safe-area-inset-bottom));
  }

  .close {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    margin: 0;
  }
</style>
