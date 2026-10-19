<script lang="ts">
  import MediaLightbox from './MediaLightbox.svelte'
  import { setMediaVisibility } from '../lib/api/moderation'
  import type { GalleryItem } from '../lib/api/media'
  import { detailImage, downloadFileName, downloadUrl, isDoubleTap, isVideoItem, videoPoster, type TapPoint, startPlayback } from '../lib/gallery/view'
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
  // Notices float over the media, so they never move the image or the controls (S2).
  const notice = $derived(hideError !== '' ? hideError : likeFailedId === item.id ? copy.likes.failed : '')

  $effect(() => {
    // An error belongs to the photo that failed to hide.
    void item.id
    hideError = ''
  })

  function autoplay(node: HTMLVideoElement) {
    void startPlayback(node)
  }

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
</script>

<MediaLightbox
  label={copy.gallery.photoAlt}
  {onClose}
  onSwipe={(direction) => (direction === 'next' ? next() : previous())}
  {onKeydown}
  {onFramePointerUp}
  {notice}
>
  {#snippet topStart()}
    {#if onHide !== undefined}
      <button class="button small ghost" type="button" onclick={hide} disabled={hiding}>{copy.detail.hide}</button>
    {/if}
  {/snippet}

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
          preload="auto"
          use:autoplay
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

  {#snippet controls()}
    <button class="button round ghost" type="button" onclick={previous} disabled={!hasPrevious} aria-label={copy.detail.previous}>‹</button>
    <a class="button primary compact" href={downloadUrl(item)} download={downloadFileName(item)}>{copy.detail.download}</a>
    <button
      class="button ghost like"
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
    <button class="button round ghost" type="button" onclick={next} disabled={!hasNext} aria-label={copy.detail.next}>›</button>
  {/snippet}
</MediaLightbox>

<style>
  .heart-burst {
    position: absolute;
    inset: 0;
    z-index: 1;
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
    box-sizing: border-box;
    height: 100%;
    display: grid;
    place-items: center;
    max-width: 28rem;
    margin: 0 auto;
    padding: 1rem;
    text-align: center;
  }

  .button.primary.compact {
    box-sizing: border-box;
    min-height: 2.75rem;
    padding: 0 1.2rem;
    white-space: nowrap;
  }

  .like {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-width: 4.25rem;
    white-space: nowrap;
  }

  .like.liked {
    color: var(--danger-soft);
  }
</style>
