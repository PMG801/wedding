import { describe, expect, it, vi } from 'vitest'
import { GuestSessionRequiredError, type GalleryItem, type GalleryPage } from '../api/media'
import { GalleryFeed, type GalleryFeedState } from './feed'

function item(n: number): GalleryItem {
  const id = `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`
  return { id, mime: 'image/jpeg', confirmedAt: n, originalUrl: `/m/o/${n}.jpg`, thumbUrl: null, detailUrl: null }
}

function page(numbers: number[], nextCursor: string | null): GalleryPage {
  return { items: numbers.map(item), nextCursor }
}

function setup(fetchPage: (cursor: string | null) => Promise<GalleryPage>) {
  const states: GalleryFeedState[] = []
  const feed = new GalleryFeed({ fetchPage, onChange: (state) => states.push(state) })
  return { feed, states, last: () => states.at(-1)! }
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
