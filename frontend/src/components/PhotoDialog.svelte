<script lang="ts">
  import { setMediaVisibility } from '../lib/api/moderation'
  import type { GalleryItem } from '../lib/api/media'
  import { detailImage, downloadFileName, downloadUrl, isVideoItem, videoPoster } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  let {
    items,
    index,
    hasMore,
    onNavigate,
    onClose,
    onHide
  }: {
    items: GalleryItem[]
    index: number
    hasMore: boolean
    onNavigate: (index: number) => void
    onClose: () => void
    /** Present only for an administrator; called after the item was hidden on the server. */
    onHide?: (id: string) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let touchStartX: number | null = null
  let failedVideoId = $state<string | null>(null)
  let hiding = $state(false)
  let hideError = $state('')

  const item = $derived(items[index])
  const image = $derived(detailImage(item))
  const video = $derived(isVideoItem(item))
  const hasPrevious = $derived(index > 0)
  const hasNext = $derived(index < items.length - 1 || hasMore)

  $effect(() => {
    if (dialog !== undefined && !dialog.open) dialog.showModal()
  })

  $effect(() => {
    // An error belongs to the photo that failed to hide.
    void item.id
    hideError = ''
  })

  function previous() {
    if (hasPrevious) onNavigate(index - 1)
  }

  function next() {
    if (index < items.length - 1) onNavigate(index + 1)
  }

  async function hide() {
    if (hiding || !confirm(copy.detail.hideConfirm)) return
    hiding = true
    hideError = ''
    try {
      await setMediaVisibility(item.id, 'hidden')
    } catch {
      hideError = copy.detail.hideFailed
      return
    } finally {
      hiding = false
    }
    onHide?.(item.id)
    onClose()
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
    {#if video}
      {#if failedVideoId === item.id}
        <p class="no-preview">{copy.detail.videoUnsupported}</p>
      {:else}
        {#key item.id}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video
            src={item.originalUrl}
            poster={videoPoster(item)}
            controls
            playsinline
            preload="metadata"
            onerror={() => (failedVideoId = item.id)}
          ></video>
        {/key}
      {/if}
    {:else if image !== null}
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
    {#if onHide !== undefined}
      <button class="button" type="button" onclick={hide} disabled={hiding}>{copy.detail.hide}</button>
    {/if}
    <button class="button round" type="button" onclick={next} disabled={!hasNext} aria-label={copy.detail.next}>›</button>
    {#if hideError !== ''}
      <p class="hide-error" role="alert">{hideError}</p>
    {/if}
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

  .frame img,
  .frame video {
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
    flex-wrap: wrap;
    gap: 1rem;
    padding: 0.75rem 1rem calc(0.75rem + env(safe-area-inset-bottom));
  }

  .hide-error {
    width: 100%;
    margin: 0;
    text-align: center;
    color: var(--danger);
  }

  .close {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    margin: 0;
  }
</style>
