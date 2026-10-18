<script lang="ts">
  import { onMount } from 'svelte'
  import GalleryGrid from '../components/GalleryGrid.svelte'
  import PhotoDialog from '../components/PhotoDialog.svelte'
  import PinnedSection from '../components/PinnedSection.svelte'
  import UploadPanel from '../components/UploadPanel.svelte'
  import { fetchGalleryPage, fetchTopMedia, setLike, xhrTransport } from '../lib/api/media'
  import { hasAdminSession } from '../lib/api/moderation'
  import { GalleryFeed, type GalleryFeedState } from '../lib/gallery/feed'
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

  onMount(() => {
    void feed.loadMore()
    const onHashChange = () => (route = parseRoute(location.hash))
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (busy) event.preventDefault()
    }
    void hasAdminSession().then((isAdmin) => (canHide = isAdmin))
    addEventListener('hashchange', onHashChange)
    addEventListener('beforeunload', onBeforeUnload)
    return () => {
      clearTimeout(refreshTimer)
      removeEventListener('hashchange', onHashChange)
      removeEventListener('beforeunload', onBeforeUnload)
    }
  })

  function showPhoto(index: number) {
    const item = gallery.items[index]
    if (item === undefined) return
    // Replace instead of push so browsing photos does not flood the history.
    history.replaceState(null, '', photoHash(item.id))
    route = parseRoute(location.hash)
    if (index >= gallery.items.length - 3 && gallery.hasMore) void feed.loadMore()
  }

  function closePhoto() {
    history.replaceState(null, '', `${location.pathname}${location.search}`)
    route = { name: 'home' }
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
