<script lang="ts">
  import { onMount } from 'svelte'
  import BottomNav from '../components/BottomNav.svelte'
  import GalleryGrid from '../components/GalleryGrid.svelte'
  import PhotoDialog from '../components/PhotoDialog.svelte'
  import PinnedSection from '../components/PinnedSection.svelte'
  import UploadPanel from '../components/UploadPanel.svelte'
  import { copy } from '../lib/i18n/es'
  import { fetchGalleryPage, fetchTopMedia, setLike, xhrTransport } from '../lib/api/media'
  import { hasAdminSession } from '../lib/api/moderation'
  import { GalleryFeed, type GalleryFeedState, type GalleryView } from '../lib/gallery/feed'
  import { swipeDirection, viewAfterSwipe } from '../lib/gallery/swipe'
  import { adoptDeepLink, closeViewer, moveViewer, openViewer } from '../lib/gallery/history'
  import { parseRoute, photoHash, type Route } from '../lib/gallery/view'
  import { createMediaDerivatives } from '../lib/media/poster'
  import { UploadQueue, type UploadItem } from '../lib/upload/queue'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  let uploads = $state<readonly UploadItem[]>([])
  let dismissed = $state(new Set<string>())
  let gallery = $state<GalleryFeedState>({ view: 'recent', items: [], status: 'idle', hasMore: true, likeFailedId: null })
  let route = $state<Route>(parseRoute(location.hash))
  let canHide = $state(false)
  // Height of the fixed bottom dock (strip + nav); the page keeps that much room at its end (S8).
  let dockHeight = $state(0)

  let finishedUploads = 0
  let refreshTimer: ReturnType<typeof setTimeout> | undefined

  const feed = new GalleryFeed({
    fetchPage: (cursor) => fetchGalleryPage({ cursor }),
    fetchTop: () => fetchTopMedia(),
    sendLike: (id, liked) => setLike(id, liked),
    onChange: (state) => {
      gallery = state
      if (state.status === 'session_required') onSessionLost()
    }
  })

  const queue = new UploadQueue({
    transport: xhrTransport,
    makeDerivatives: (file) => createMediaDerivatives(file),
    onChange: (items) => {
      uploads = items
      const finished = items.filter((item) => item.status === 'done').length
      if (finished > finishedUploads) {
        finishedUploads = finished
        // Batch gallery refreshes while several photos finish close together.
        clearTimeout(refreshTimer)
        refreshTimer = setTimeout(() => void feed.refresh(), 800)
      }
    }
  })

  const visibleUploads = $derived(uploads.filter((item) => !dismissed.has(item.id)))
  const busy = $derived(uploads.some((item) => item.status !== 'done' && item.status !== 'failed'))
  const openIndex = $derived.by(() => {
    const current = route
    return current.name === 'photo' ? gallery.items.findIndex((item) => item.id === current.id) : -1
  })

  const homeUrl = () => `${location.pathname}${location.search}`

  onMount(() => {
    void feed.loadMore()
    // A shared photo link still lets the back button return to the gallery (S4).
    if (route.name === 'photo') adoptDeepLink(history, homeUrl(), location.hash)
    const onHashChange = () => (route = parseRoute(location.hash))
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (busy) event.preventDefault()
    }
    void hasAdminSession().then((isAdmin) => (canHide = isAdmin))
    addEventListener('hashchange', onHashChange)
    addEventListener('popstate', onHashChange)
    addEventListener('beforeunload', onBeforeUnload)
    return () => {
      clearTimeout(refreshTimer)
      removeEventListener('hashchange', onHashChange)
      removeEventListener('popstate', onHashChange)
      removeEventListener('beforeunload', onBeforeUnload)
    }
  })

  /** Opens a photo from the grid on its own history entry, so back closes it (S4). */
  function openPhoto(id: string) {
    openViewer(history, photoHash(id))
    route = parseRoute(location.hash)
  }

  function showPhoto(index: number) {
    const item = gallery.items[index]
    if (item === undefined) return
    // Replace instead of push so browsing photos does not flood the history.
    moveViewer(history, photoHash(item.id))
    route = parseRoute(location.hash)
    if (index >= gallery.items.length - 3 && gallery.hasMore) void feed.loadMore()
  }

  function closePhoto() {
    if (route.name !== 'photo') return
    // Going back fires popstate, which brings the route home.
    if (closeViewer(history, homeUrl()) === 'replaced') route = { name: 'home' }
  }

  let swipeStart: { x: number; y: number } | null = null
  let swipeTouches = 0

  function onTouchStart(event: TouchEvent) {
    swipeTouches = Math.max(event.touches.length === 1 ? 0 : swipeTouches, event.touches.length)
    const touch = event.touches[0]
    swipeStart = event.touches.length === 1 ? { x: touch.clientX, y: touch.clientY } : null
  }

  function onTouchEnd(event: TouchEvent) {
    if (event.touches.length > 0) return
    const first = swipeStart
    const fingers = swipeTouches
    swipeStart = null
    swipeTouches = 0
    if (first === null || route.name !== 'home') return
    const touch = event.changedTouches[0]
    const direction = swipeDirection({
      dx: touch.clientX - first.x,
      dy: touch.clientY - first.y,
      maxTouches: fingers,
      zoomed: (window.visualViewport?.scale ?? 1) > 1.01,
      onVideoControls: false
    })
    const next = direction === null ? null : viewAfterSwipe(gallery.view, direction)
    if (next !== null) selectView(next)
  }

  function dismissFinished() {
    dismissed = new Set([...dismissed, ...uploads.filter((item) => item.status === 'done').map((item) => item.id)])
  }

  function scrollToGallery(smooth: boolean) {
    const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    document.getElementById('gallery')?.scrollIntoView({ behavior: smooth && !reduced ? 'smooth' : 'auto', block: 'start' })
  }

  /**
   * The active tab scrolls back to the gallery top. Another tab switches the view and starts
   * again at the top, instantly, so the window (D4) and the scroll position agree.
   */
  function selectView(view: GalleryView) {
    if (view === gallery.view) return scrollToGallery(true)
    scrollToGallery(false)
    void feed.setView(view)
  }
</script>

<PinnedSection onSessionLost={onSessionLost} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- Horizontal swipe switches between Recientes and Más gustadas; vertical scroll stays native. -->
<div class="swipe-area" ontouchstart={onTouchStart} ontouchend={onTouchEnd} ontouchcancel={() => (swipeStart = null)}>
  <GalleryGrid
    feed={gallery}
    onLoadMore={() => void feed.loadMore()}
    onOpen={openPhoto}
    onLike={(id) => void feed.toggleLike(id)}
  />
</div>

{#if openIndex >= 0}
  <PhotoDialog
    items={gallery.items}
    index={openIndex}
    hasMore={gallery.hasMore}
    onNavigate={showPhoto}
    onClose={closePhoto}
    onLike={(id) => void feed.toggleLike(id)}
    likeFailedId={gallery.likeFailedId}
    onHide={canHide ? (id) => feed.remove(id) : undefined}
  />
{/if}

<div class="dock" bind:clientHeight={dockHeight}>
  <UploadPanel
    items={visibleUploads}
    onRetry={(id) => queue.retry(id)}
    onDismissFinished={dismissFinished}
  />
  <BottomNav view={gallery.view} onViewChange={selectView} onFiles={(files) => queue.add(files)} />
</div>
<div class="dock-room" aria-hidden="true" style:height="{dockHeight}px"></div>

<style>
  .swipe-area {
    touch-action: pan-y pinch-zoom;
  }

  /* Fixed under the page: the strip above the nav, both in the same 960px column. */
  .dock {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 20;
  }
</style>
