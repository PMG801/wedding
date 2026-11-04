<script lang="ts">
  import { onMount } from 'svelte'
  import BackToTop from '../components/BackToTop.svelte'
  import BottomNav from '../components/BottomNav.svelte'
  import CarouselStrip from '../components/CarouselStrip.svelte'
  import GalleryGrid from '../components/GalleryGrid.svelte'
  import PhotoDialog from '../components/PhotoDialog.svelte'
  import PinnedSection from '../components/PinnedSection.svelte'
  import UploadPanel from '../components/UploadPanel.svelte'
  import { copy } from '../lib/i18n/es'
  import { fetchCarousels, type CarouselPeriod } from '../lib/api/carousels'
  import { fetchGalleryPage, fetchTopMedia, GuestSessionRequiredError, setLike, xhrTransport } from '../lib/api/media'
  import { tusTransport } from '../lib/api/uploads'
  import { hasAdminSession } from '../lib/api/moderation'
  import { periodLabel } from '../lib/carousel/label'
  import { RETRY_DELAY_MS, refreshDelayMs } from '../lib/carousel/rotation'
  import { GalleryFeed, type GalleryFeedState, type GalleryView } from '../lib/gallery/feed'
  import { swipeDirection, viewAfterSwipe } from '../lib/gallery/swipe'
  import { adoptDeepLink, closeViewer, moveViewer, openViewer } from '../lib/gallery/history'
  import { albumHash, parseRoute, photoHash, storyHash, type Route } from '../lib/gallery/view'
  import { createMediaDerivatives } from '../lib/media/poster'
  import { UploadQueue, type UploadItem } from '../lib/upload/queue'

  let { onSessionLost }: { onSessionLost: () => void } = $props()

  let uploads = $state<readonly UploadItem[]>([])
  let dismissed = $state(new Set<string>())
  let gallery = $state<GalleryFeedState>({ view: 'recent', order: 'newest', kind: 'all', items: [], status: 'idle', hasMore: true, likeFailedId: null })
  let route = $state<Route>(parseRoute(location.hash))
  let canHide = $state(false)
  // Height of the fixed bottom dock (strip + nav); the page keeps that much room at its end (S8).
  let dockHeight = $state(0)

  // Story strip (S8): periods with media, refetched at each rotation slot. Empty hides it.
  let carousel = $state<CarouselPeriod[]>([])
  let carouselTimer: ReturnType<typeof setTimeout> | undefined

  let finishedUploads = 0
  let refreshTimer: ReturnType<typeof setTimeout> | undefined

  const feed = new GalleryFeed({
    fetchPage: (cursor, order, kind) => fetchGalleryPage({ cursor, order, kind }),
    fetchTop: () => fetchTopMedia(),
    sendLike: (id, liked) => setLike(id, liked),
    onChange: (state) => {
      gallery = state
      if (state.status === 'session_required') onSessionLost()
    }
  })

  const queue = new UploadQueue({
    transport: xhrTransport,
    originalTransport: tusTransport,
    makeDerivatives: (file) => createMediaDerivatives(file),
    // Loaded on the first upload only: the EXIF/ISO-BMFF reader stays out of the entry chunk.
    captureTime: (file) => import('../lib/media/captureTime').then(({ readCaptureTime }) => readCaptureTime(file)),
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

  const storyPeriod = $derived.by(() => {
    const current = route
    return current.name === 'story' ? (carousel.find((period) => period.id === current.id) ?? null) : null
  })
  const albumLabel = $derived.by(() => {
    const current = route
    return current.name === 'album' ? carouselLabel(current.id) : null
  })

  const homeUrl = () => `${location.pathname}${location.search}`

  function carouselLabel(id: number): string | null {
    const period = carousel.find((candidate) => candidate.id === id)
    return period === undefined ? null : periodLabel(period.start, period.end)
  }

  /** Loads the strip and schedules the next refetch at the rotation slot (S8). */
  async function loadCarousel() {
    clearTimeout(carouselTimer)
    try {
      const data = await fetchCarousels()
      carousel = data.periods
      carouselTimer = setTimeout(() => void loadCarousel(), refreshDelayMs(data.rotatesAt, Date.now()))
    } catch (error) {
      // The strip stays hidden; a lost session is reported by the gallery itself.
      carousel = []
      if (!(error instanceof GuestSessionRequiredError)) carouselTimer = setTimeout(() => void loadCarousel(), RETRY_DELAY_MS)
    }
  }

  onMount(() => {
    void feed.loadMore()
    void loadCarousel()
    // A shared photo, story or album link still lets the back button return home (S4).
    if (route.name !== 'home') adoptDeepLink(history, homeUrl(), location.hash)
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
      clearTimeout(carouselTimer)
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

  /** Opens a period's story on its own history entry, so back closes it. */
  function openStory(id: number) {
    openViewer(history, storyHash(id))
    route = parseRoute(location.hash)
  }

  /** From a story, the album replaces the story entry: back from the album returns home. */
  function openAlbum(id: number) {
    moveViewer(history, albumHash(id))
    route = parseRoute(location.hash)
  }

  /** Closes a story or the album: back when the app pushed the entry, otherwise home in place. */
  function closeOverlay() {
    if (route.name !== 'story' && route.name !== 'album') return
    if (closeViewer(history, homeUrl()) === 'replaced') route = { name: 'home' }
  }

  /** The story's dialog close event must only close a story, never an album that replaced it. */
  function closeStory() {
    if (route.name === 'story') closeOverlay()
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

  /** A new order or filter starts again at the top, instantly, like a view switch. */
  function restartAtTop(change: Promise<void>) {
    scrollToGallery(false)
    void change
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

<CarouselStrip periods={carousel} onOpen={openStory} />

<PinnedSection onSessionLost={onSessionLost} />

<!-- svelte-ignore a11y_no_static_element_interactions -->
<!-- Horizontal swipe switches between Recientes and Más gustadas; vertical scroll stays native. -->
<div class="swipe-area" ontouchstart={onTouchStart} ontouchend={onTouchEnd} ontouchcancel={() => (swipeStart = null)}>
  <GalleryGrid
    feed={gallery}
    onLoadMore={() => void feed.loadMore()}
    onOpen={openPhoto}
    onLike={(id) => void feed.toggleLike(id)}
    onKindChange={(kind) => restartAtTop(feed.setKind(kind))}
    onOrderChange={(order) => restartAtTop(feed.setOrder(order))}
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

{#if storyPeriod !== null}
  {#await import('../components/StoryViewer.svelte') then { default: StoryViewer }}
    <StoryViewer
      items={storyPeriod.items}
      label={periodLabel(storyPeriod.start, storyPeriod.end) ?? copy.carousel.heading}
      onClose={closeStory}
      onAlbum={() => openAlbum(storyPeriod.id)}
    />
  {/await}
{/if}

{#if route.name === 'album'}
  <!-- The album covers the home page and stays above it; the dock keeps showing upload progress. -->
  <div class="album-layer" style:bottom="{dockHeight}px">
    {#await import('../components/AlbumSection.svelte')}
      <p class="notice" role="status">{copy.gallery.loading}</p>
    {:then { default: AlbumSection }}
      <AlbumSection periodId={route.id} label={albumLabel} onBack={closeOverlay} onSessionLost={onSessionLost} />
    {:catch}
      <p class="notice" role="alert">{copy.carousel.albumError}</p>
    {/await}
  </div>
{/if}

<div class="dock" bind:clientHeight={dockHeight}>
  <UploadPanel
    items={visibleUploads}
    onRetry={(id) => queue.retry(id)}
    onDismissFinished={dismissFinished}
  />
  <BottomNav view={gallery.view} onViewChange={selectView} onFiles={(files) => queue.add(files)} />
</div>
<BackToTop offset={dockHeight} />
<div class="dock-room" aria-hidden="true" style:height="{dockHeight}px"></div>

<style>
  .swipe-area {
    touch-action: pan-y pinch-zoom;
  }

  /* The album layer sits between the home page and the dock; its bottom offset is the dock height. */
  .album-layer {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    z-index: 15;
  }

  /* Fixed under the page: the strip above the nav, both in the same 960px column. */
  .dock {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 20;
    /* The room above the raised upload button stays transparent to taps on the gallery. */
    pointer-events: none;
  }

  .dock > :global(*) {
    pointer-events: auto;
  }
</style>
