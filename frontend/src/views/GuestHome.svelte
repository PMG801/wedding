<script lang="ts">
  import { onMount } from 'svelte'
  import GalleryGrid from '../components/GalleryGrid.svelte'
  import PhotoDialog from '../components/PhotoDialog.svelte'
  import PinnedSection from '../components/PinnedSection.svelte'
  import UploadPanel from '../components/UploadPanel.svelte'
  import { fetchGalleryPage, fetchTopMedia, setLike, xhrTransport } from '../lib/api/media'
  import { hasAdminSession } from '../lib/api/moderation'
  import { GalleryFeed, type GalleryFeedState } from '../lib/gallery/feed'
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

  function dismissFinished() {
    dismissed = new Set([...dismissed, ...uploads.filter((item) => item.status === 'done').map((item) => item.id)])
  }
</script>

<UploadPanel
  items={visibleUploads}
  onFiles={(files) => queue.add(files)}
  onRetry={(id) => queue.retry(id)}
  onDismissFinished={dismissFinished}
/>

<PinnedSection onSessionLost={onSessionLost} />

<GalleryGrid
  feed={gallery}
  onLoadMore={() => void feed.loadMore()}
  onViewChange={(view) => void feed.setView(view)}
  onOpen={openPhoto}
  onLike={(id) => void feed.toggleLike(id)}
/>

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
