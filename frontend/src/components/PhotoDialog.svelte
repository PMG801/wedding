<script lang="ts">
  import { setMediaVisibility } from '../lib/api/moderation'
  import type { GalleryItem } from '../lib/api/media'
  import { detailImage, downloadFileName, downloadUrl, isDoubleTap, isVideoItem, videoPoster, type TapPoint } from '../lib/gallery/view'
  import { copy } from '../lib/i18n/es'

  let {
    items,
    index,
    hasMore,
    onNavigate,
    onClose,
    onLike,
    likeFailedId,
    onHide
  }: {
    items: GalleryItem[]
    index: number
    hasMore: boolean
    onNavigate: (index: number) => void
    onClose: () => void
    onLike: (id: string) => void
    /** Id of the photo whose last like change was rolled back, if any. */
    likeFailedId: string | null
    /** Present only for an administrator; called after the item was hidden on the server. */
    onHide?: (id: string) => void
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let touchStartX: number | null = null
  let failedVideoId = $state<string | null>(null)
  let hiding = $state(false)
  let hideError = $state('')
  let lastTap: TapPoint | null = null
  let heartVisible = $state(false)
  let heartTimer: ReturnType<typeof setTimeout> | undefined

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

  function onFramePointerUp(event: PointerEvent) {
    const target = event.target as HTMLElement
    // Leave the bottom strip of a video to its native controls.
    if (target instanceof HTMLVideoElement) {
      const rect = target.getBoundingClientRect()
      if (event.clientY > rect.bottom - 56) {
        lastTap = null
        return
      }
    }
    const tap = { time: event.timeStamp, x: event.clientX, y: event.clientY }
    if (!isDoubleTap(lastTap, tap)) {
      lastTap = tap
      return
    }
    lastTap = null
    // A double tap only adds a like; removing one stays on the button.
    if (!item.likedByMe) onLike(item.id)
    heartVisible = true
    clearTimeout(heartTimer)
    heartTimer = setTimeout(() => (heartVisible = false), 700)
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
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="frame" onpointerup={onFramePointerUp}>
    {#if heartVisible}
      <span class="heart-burst" aria-hidden="true">♥</span>
    {/if}
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
    <button
      class="button like"
      class:liked={item.likedByMe}
      type="button"
      aria-label={copy.likes.like}
      aria-pressed={item.likedByMe}
      title={item.likedByMe ? copy.likes.unlike : copy.likes.like}
      onclick={() => onLike(item.id)}
    >
      <span aria-hidden="true">{item.likedByMe ? '♥' : '♡'}</span>
      <span aria-hidden="true">{item.likeCount}</span>
    </button>
    {#if onHide !== undefined}
      <button class="button" type="button" onclick={hide} disabled={hiding}>{copy.detail.hide}</button>
    {/if}
    <button class="button round" type="button" onclick={next} disabled={!hasNext} aria-label={copy.detail.next}>›</button>
    {#if likeFailedId === item.id}
      <p class="hide-error" role="alert">{copy.likes.failed}</p>
    {/if}
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
    position: relative;
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

  .frame img {
    user-select: none;
    -webkit-user-select: none;
  }

  .heart-burst {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-size: 6rem;
    color: var(--danger-soft);
    pointer-events: none;
    animation: heart-pop 700ms ease-out forwards;
  }

  @keyframes heart-pop {
    0% { opacity: 0; transform: scale(0.5); }
    30% { opacity: 1; transform: scale(1.1); }
    100% { opacity: 0; transform: scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    .heart-burst { animation-duration: 1ms; }
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

  .like {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .like.liked {
    color: var(--danger-soft);
  }

  .close {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    margin: 0;
  }
</style>
