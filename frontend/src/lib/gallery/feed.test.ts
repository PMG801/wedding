import { describe, expect, it, vi } from 'vitest'
import { GuestSessionRequiredError, type GalleryItem, type GalleryPage, type LikeResult } from '../api/media'
import { GalleryFeed, type GalleryFeedState } from './feed'

function item(n: number): GalleryItem {
  const id = `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
  return {
    id,
    mime: 'image/jpeg',
    confirmedAt: n,
    originalUrl: `/m/o/${n}.jpg`,
    thumbUrl: null,
    detailUrl: null,
    likeCount: 0,
    likedByMe: false
  }
}

function page(numbers: number[], nextCursor: string | null): GalleryPage {
  return { items: numbers.map(item), nextCursor }
}

interface SetupOptions {
  fetchTop?: () => Promise<GalleryItem[]>
  sendLike?: (id: string, liked: boolean) => Promise<LikeResult>
}

function setup(fetchPage: (cursor: string | null) => Promise<GalleryPage>, options: SetupOptions = {}) {
  const states: GalleryFeedState[] = []
  const feed = new GalleryFeed({
    fetchPage,
    fetchTop: options.fetchTop ?? (async () => []),
    sendLike: options.sendLike ?? (async (id, liked) => ({ id, likeCount: liked ? 1 : 0, likedByMe: liked })),
    onChange: (state) => states.push(state)
  })
  return { feed, states, last: () => states.at(-1)! }
}

/** A promise the test settles by hand, to observe the state while a request is in flight. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('GalleryFeed', () => {
  it('loads pages in order with the cursor and stops at the last page', async () => {
    const fetchPage = vi.fn(async (cursor: string | null) => (cursor === null ? page([5, 4], 'c1') : page([3], null)))
    const { feed, last } = setup(fetchPage)

    await feed.loadMore()
    await feed.loadMore()
    await feed.loadMore()

    expect(fetchPage.mock.calls).toEqual([[null], ['c1']])
    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([5, 4, 3])
    expect(last()).toEqual(expect.objectContaining({ status: 'idle', hasMore: false }))
  })

  it('ignores a second load while one is in flight', async () => {
    let resolve!: (value: GalleryPage) => void
    const fetchPage = vi.fn(() => new Promise<GalleryPage>((r) => (resolve = r)))
    const { feed, last } = setup(fetchPage)

    const first = feed.loadMore()
    void feed.loadMore()
    expect(last().status).toBe('loading')
    resolve(page([1], null))
    await first
    expect(fetchPage).toHaveBeenCalledTimes(1)
  })

  it('prepends new photos on refresh without duplicating loaded ones', async () => {
    const responses = [page([5, 4], 'c1'), page([7, 6, 5], 'x')]
    const { feed, last } = setup(async () => responses.shift()!)

    await feed.loadMore()
    await feed.refresh()

    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([7, 6, 5, 4])
    expect(last().hasMore).toBe(true)
  })

  it('updates derivative URLs of photos already shown when refreshing', async () => {
    const withThumb = { ...item(5), thumbUrl: '/m/d/5_t.jpg' }
    const responses = [page([5], null), { items: [withThumb], nextCursor: null }]
    const { feed, last } = setup(async () => responses.shift()!)

    await feed.loadMore()
    await feed.refresh()

    expect(last().items).toEqual([withThumb])
  })

  it('restarts from the newest page when a refresh has no overlap', async () => {
    const responses = [page([5, 4], 'c1'), page([60, 59], 'c9')]
    const fetchPage = vi.fn(async () => responses.shift()!)
    const { feed, last } = setup(fetchPage)

    await feed.loadMore()
    await feed.refresh()
    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([60, 59])

    responses.push(page([58], null))
    await feed.loadMore()
    expect(fetchPage).toHaveBeenLastCalledWith('c9')
  })

  it('reports errors and a missing guest session, and can recover', async () => {
    const fetchPage = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockRejectedValueOnce(new GuestSessionRequiredError())
      .mockResolvedValueOnce(page([1], null))
    const { feed, last } = setup(fetchPage)

    await feed.loadMore()
    expect(last().status).toBe('error')
    await feed.loadMore()
    expect(last().status).toBe('session_required')
    await feed.loadMore()
    expect(last()).toEqual(expect.objectContaining({ status: 'idle', items: [item(1)] }))
  })

  it('removes one photo without refetching and emits the change', async () => {
    const fetchPage = vi.fn(async () => page([3, 2, 1], null))
    const { feed, last, states } = setup(fetchPage)

    await feed.loadMore()
    const emitted = states.length
    feed.remove(item(2).id)

    expect(fetchPage).toHaveBeenCalledTimes(1)
    expect(states.length).toBe(emitted + 1)
    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([3, 1])
  })

  it('ignores removing an unknown photo', async () => {
    const { feed, last, states } = setup(async () => page([1], null))

    await feed.loadMore()
    const emitted = states.length
    feed.remove(item(99).id)

    expect(states.length).toBe(emitted)
    expect(last().items).toEqual([item(1)])
  })
})

describe('GalleryFeed likes', () => {
  it('shows the like at once, then keeps the server-confirmed state', async () => {
    const reply = deferred<LikeResult>()
    const sendLike = vi.fn(() => reply.promise)
    const { feed, last } = setup(async () => page([1], null), { sendLike })
    await feed.loadMore()

    const pending = feed.toggleLike(item(1).id)
    expect(sendLike).toHaveBeenCalledWith(item(1).id, true)
    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 1, likedByMe: true }))

    reply.resolve({ id: item(1).id, likeCount: 5, likedByMe: true })
    await pending
    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 5, likedByMe: true }))
    expect(last().likeFailedId).toBeNull()
  })

  it('rolls back to the previous state when the server rejects the change and flags the photo', async () => {
    const sendLike = vi.fn().mockRejectedValue(new Error('offline'))
    const { feed, last } = setup(async () => page([1], null), { sendLike })
    await feed.loadMore()

    await feed.toggleLike(item(1).id)

    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 0, likedByMe: false }))
    expect(last().likeFailedId).toBe(item(1).id)
    expect(last().status).toBe('idle')
  })

  it('unlikes optimistically and never drops the count below zero', async () => {
    const liked = { ...item(1), likeCount: 1, likedByMe: true }
    const inconsistent = { ...item(2), likeCount: 0, likedByMe: true }
    const reply = deferred<LikeResult>()
    const sendLike = vi.fn((id: string) => (id === liked.id ? reply.promise : Promise.resolve({ id, likeCount: 0, likedByMe: false })))
    const { feed, last } = setup(async () => ({ items: [liked, inconsistent], nextCursor: null }), { sendLike })
    await feed.loadMore()

    const pending = feed.toggleLike(liked.id)
    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 0, likedByMe: false }))
    reply.resolve({ id: liked.id, likeCount: 0, likedByMe: false })
    await pending

    await feed.toggleLike(inconsistent.id)
    expect(last().items[1]).toEqual(expect.objectContaining({ likeCount: 0, likedByMe: false }))
  })

  it('ignores a second toggle of the same photo while its request is in flight', async () => {
    const reply = deferred<LikeResult>()
    const sendLike = vi.fn(() => reply.promise)
    const { feed, last } = setup(async () => page([1], null), { sendLike })
    await feed.loadMore()

    const first = feed.toggleLike(item(1).id)
    await feed.toggleLike(item(1).id)
    expect(sendLike).toHaveBeenCalledTimes(1)
    expect(last().items[0].likeCount).toBe(1)

    reply.resolve({ id: item(1).id, likeCount: 1, likedByMe: true })
    await first
    await feed.toggleLike(item(1).id)
    expect(sendLike).toHaveBeenLastCalledWith(item(1).id, false)
  })

  it('keeps other photos independent while one is in flight', async () => {
    const sendLike = vi.fn(async (id: string, liked: boolean) => ({ id, likeCount: liked ? 1 : 0, likedByMe: liked }))
    const { feed, last } = setup(async () => page([2, 1], null), { sendLike })
    await feed.loadMore()

    await Promise.all([feed.toggleLike(item(1).id), feed.toggleLike(item(2).id)])
    expect(sendLike).toHaveBeenCalledTimes(2)
    expect(last().items.map((entry) => entry.likedByMe)).toEqual([true, true])
  })

  it('reports a lost guest session from a like as a session_required status', async () => {
    const sendLike = vi.fn().mockRejectedValue(new GuestSessionRequiredError())
    const { feed, last } = setup(async () => page([1], null), { sendLike })
    await feed.loadMore()

    await feed.toggleLike(item(1).id)
    expect(last().status).toBe('session_required')
    expect(last().items[0].likedByMe).toBe(false)
  })

  it('does nothing for a photo that is not listed', async () => {
    const sendLike = vi.fn()
    const { feed } = setup(async () => page([1], null), { sendLike })
    await feed.loadMore()

    await feed.toggleLike(item(99).id)
    expect(sendLike).not.toHaveBeenCalled()
  })
})

describe('GalleryFeed ranking view', () => {
  it('loads the ranking once with no pagination and keeps the chronological feed intact', async () => {
    const fetchPage = vi.fn(async () => page([3, 2], 'c1'))
    const fetchTop = vi.fn(async () => [{ ...item(9), likeCount: 4, likedByMe: false }, { ...item(8), likeCount: 1 }])
    const { feed, last } = setup(fetchPage, { fetchTop })
    await feed.loadMore()

    await feed.setView('top')
    expect(fetchTop).toHaveBeenCalledTimes(1)
    expect(last()).toEqual(expect.objectContaining({ view: 'top', status: 'idle', hasMore: false }))
    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([9, 8])

    await feed.loadMore()
    expect(fetchPage).toHaveBeenCalledTimes(1)
    expect(fetchTop).toHaveBeenCalledTimes(1)

    await feed.setView('recent')
    expect(last()).toEqual(expect.objectContaining({ view: 'recent', hasMore: true }))
    expect(last().items.map((entry) => entry.confirmedAt)).toEqual([3, 2])
  })

  it('shows an empty ranking as an empty list without any page to load', async () => {
    const { feed, last } = setup(async () => page([], null), { fetchTop: async () => [] })
    await feed.setView('top')
    expect(last()).toEqual(expect.objectContaining({ view: 'top', status: 'idle', items: [], hasMore: false }))
  })

  it('reports ranking failures and retries them on the next load request', async () => {
    const fetchTop = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([item(5)])
    const { feed, last } = setup(async () => page([], null), { fetchTop })

    await feed.setView('top')
    expect(last()).toEqual(expect.objectContaining({ status: 'error', items: [] }))
    await feed.loadMore()
    expect(fetchTop).toHaveBeenCalledTimes(2)
    expect(last()).toEqual(expect.objectContaining({ status: 'idle', items: [item(5)] }))
  })

  it('applies likes to the ranking and to the chronological feed in one place', async () => {
    const sendLike = vi.fn(async (id: string, liked: boolean) => ({ id, likeCount: liked ? 2 : 1, likedByMe: liked }))
    const top = { ...item(1), likeCount: 1, likedByMe: false }
    const { feed, last } = setup(async () => page([1], null), { fetchTop: async () => [top], sendLike })
    await feed.loadMore()
    await feed.setView('top')

    await feed.toggleLike(top.id)
    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 2, likedByMe: true }))

    await feed.setView('recent')
    expect(last().items[0]).toEqual(expect.objectContaining({ likeCount: 2, likedByMe: true }))
  })

  it('removes a hidden photo from the ranking too', async () => {
    const top = { ...item(1), likeCount: 1 }
    const { feed, last } = setup(async () => page([], null), { fetchTop: async () => [top, item(2)] })
    await feed.setView('top')

    feed.remove(top.id)
    expect(last().items.map((entry) => entry.id)).toEqual([item(2).id])
  })
})
