<script lang="ts">
  import type { Snippet } from 'svelte'
  import { swipeDirection } from '../lib/gallery/swipe'
  import { copy } from '../lib/i18n/es'

  /**
   * Full-screen viewer shell shared by gallery photos and pinned images (S7).
   * Layout (S2, S3): a fixed top bar, a media area that always contains the
   * media whole, and an optional controls bar of fixed height. Media never
   * covers the bars and notices float over the media without moving it.
   */
  let {
    label,
    onClose,
    onSwipe,
    onKeydown,
    onFramePointerUp,
    notice = '',
    topStart,
    controls,
    children
  }: {
    label: string
    onClose: () => void
    onSwipe?: (direction: 'previous' | 'next') => void
    onKeydown?: (event: KeyboardEvent) => void
    onFramePointerUp?: (event: PointerEvent) => void
    /** Error or status text shown over the media; empty for none. */
    notice?: string
    topStart?: Snippet
    controls?: Snippet
    children: Snippet
  } = $props()

  let dialog = $state<HTMLDialogElement>()
  let closeButton = $state<HTMLButtonElement>()
  let start: { x: number; y: number; onVideoControls: boolean } | null = null
  let maxTouches = 0

  $effect(() => {
    if (dialog === undefined || dialog.open) return
    dialog.showModal()
    // Start on the harmless action, never on an admin action such as «Ocultar».
    closeButton?.focus()
  })

  /** The bottom strip of a video belongs to its native controls (seek bar). */
  function onVideoControls(target: EventTarget | null, y: number): boolean {
    if (!(target instanceof HTMLVideoElement)) return false
    return y > target.getBoundingClientRect().bottom - 56
  }

  function onTouchStart(event: TouchEvent) {
    maxTouches = Math.max(maxTouches, event.touches.length)
    if (event.touches.length !== 1) return
    const touch = event.touches[0]
    start = { x: touch.clientX, y: touch.clientY, onVideoControls: onVideoControls(event.target, touch.clientY) }
  }

  function onTouchMove(event: TouchEvent) {
    maxTouches = Math.max(maxTouches, event.touches.length)
  }

  function onTouchEnd(event: TouchEvent) {
    // Wait until the last finger lifts so a pinch is judged as one gesture.
    if (event.touches.length > 0) return
    const first = start
    const fingers = maxTouches
    start = null
    maxTouches = 0
    if (first === null || onSwipe === undefined) return
    const touch = event.changedTouches[0]
    const direction = swipeDirection({
      dx: touch.clientX - first.x,
      dy: touch.clientY - first.y,
      maxTouches: fingers,
      zoomed: (window.visualViewport?.scale ?? 1) > 1.01,
      onVideoControls: first.onVideoControls
    })
    if (direction !== null) onSwipe(direction)
  }
</script>

<dialog
  bind:this={dialog}
  class="lightbox"
  class:has-controls={controls !== undefined}
  aria-label={label}
  onclose={onClose}
  onkeydown={onKeydown}
  ontouchstart={onTouchStart}
  ontouchmove={onTouchMove}
  ontouchend={onTouchEnd}
  ontouchcancel={() => ((start = null), (maxTouches = 0))}
>
  <div class="top">
    <div class="top-start">
      {@render topStart?.()}
    </div>
    <form method="dialog">
      <button bind:this={closeButton} class="button round ghost" type="submit" aria-label={copy.detail.close}>✕</button>
    </form>
  </div>

  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="frame" onpointerup={onFramePointerUp}>
    {@render children()}
    {#if notice !== ''}
      <p class="notice-toast" role="alert">{notice}</p>
    {/if}
  </div>

  {#if controls !== undefined}
    <div class="controls">
      {@render controls()}
    </div>
  {/if}
</dialog>

<style>
  .lightbox {
    box-sizing: border-box;
    width: 100vw;
    max-width: 100vw;
    height: 100dvh;
    max-height: 100dvh;
    margin: 0;
    padding: 0;
    border: none;
    background: #1d1a17;
    color: var(--paper);
    overscroll-behavior: contain;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: 3.5rem minmax(0, 1fr);
    grid-template-areas: 'top' 'frame';
  }

  .lightbox.has-controls {
    grid-template-rows: 3.5rem minmax(0, 1fr) 4.25rem;
    grid-template-areas: 'top' 'frame' 'controls';
  }

  .lightbox::backdrop {
    background: rgb(20 17 15 / 0.9);
  }

  /* Short open transition only; the closed dialog is display: none, so nothing animates on close (S10). */
  .lightbox[open] {
    animation: viewer-in 180ms ease-out;
  }

  @keyframes viewer-in {
    from {
      opacity: 0;
      transform: scale(0.98);
    }
  }

  .top {
    grid-area: top;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    padding: 0 0.75rem;
  }

  .top form {
    margin: 0;
  }

  .top-start {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .frame {
    grid-area: frame;
    position: relative;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  /* Any aspect ratio is letterboxed inside the media area, never past it (S3). */
  /* The media element is re-created on each navigation, so this fades every photo in (S10). */
  .frame :global(img),
  .frame :global(video) {
    display: block;
    width: 100%;
    height: 100%;
    object-fit: contain;
    touch-action: pan-y pinch-zoom;
    animation: media-in 150ms ease-out;
  }

  @keyframes media-in {
    from {
      opacity: 0;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .lightbox[open],
    .frame :global(img),
    .frame :global(video) {
      animation-duration: 0.01ms;
    }
  }

  .frame :global(img) {
    user-select: none;
    -webkit-user-select: none;
  }

  .notice-toast {
    position: absolute;
    left: 50%;
    bottom: 0.75rem;
    transform: translateX(-50%);
    width: max-content;
    max-width: calc(100% - 2rem);
    margin: 0;
    padding: 0.4rem 0.9rem;
    border-radius: 999px;
    background: var(--paper);
    color: var(--danger);
    font-size: 0.9rem;
    text-align: center;
    pointer-events: none;
  }

  .controls {
    grid-area: controls;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: nowrap;
    gap: 0.5rem;
    min-width: 0;
    padding: 0 0.75rem;
  }

  .lightbox :global(.button.ghost) {
    border-color: rgb(255 253 248 / 0.35);
    background: rgb(255 253 248 / 0.08);
    color: var(--paper);
  }

  /* A phone held sideways keeps the full height for the media (S3). */
  @media (orientation: landscape) and (max-height: 520px) {
    .lightbox,
    .lightbox.has-controls {
      grid-template-columns: minmax(0, 1fr) auto;
      grid-template-rows: auto minmax(0, 1fr);
      grid-template-areas: 'frame top' 'frame controls';
    }

    .lightbox:not(.has-controls) {
      grid-template-areas: 'frame top' 'frame .';
    }

    .top {
      height: 3.5rem;
    }

    .controls {
      flex-direction: column;
      justify-content: center;
      padding: 0.5rem 0.75rem;
    }
  }
</style>
