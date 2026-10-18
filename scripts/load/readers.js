// T5 readers: gallery browsing plus Range reads of existing videos, meant to run
// while scripts/load/slow-upload.sh (T3) is in flight.
//
// - gallery VUs walk GET /api/media with cursor pagination (p95 is the gallery API metric);
// - range VUs read random byte windows of seeded videos through /m/o/<storage_name>
//   and assert 206, the exact Content-Range and the exact Content-Length.
//
// Videos are discovered once in setup() from the gallery, so they must already be
// confirmed (run slow-upload.sh once beforehand, or let an earlier run finish).
// See scripts/load/readers.sh for the environment variables and the k6 wrapper.
import http from 'k6/http'
import { check, fail, sleep } from 'k6'
import { Counter, Rate } from 'k6/metrics'

const num = (name, fallback) => {
  const raw = __ENV[name]
  if (raw === undefined || raw === '') return fallback
  const value = Number(raw)
  if (!Number.isFinite(value) || value < 0) fail(`${name} must be a non-negative number, got "${raw}"`)
  return value
}

const BASE_URL = (__ENV.LOAD_BASE_URL || '').replace(/\/$/, '')
const EVENT_TOKEN = __ENV.LOAD_EVENT_TOKEN || ''
if (!BASE_URL || !EVENT_TOKEN) fail('LOAD_BASE_URL and LOAD_EVENT_TOKEN are required')

const GALLERY_VUS = Math.floor(num('LOAD_GALLERY_VUS', 30))
const RANGE_VUS = Math.floor(num('LOAD_RANGE_VUS', 10))
const DURATION = __ENV.LOAD_DURATION || '10m'
const GALLERY_LIMIT = Math.min(100, Math.max(1, Math.floor(num('LOAD_GALLERY_LIMIT', 24))))
const GALLERY_PAGES = Math.max(1, Math.floor(num('LOAD_GALLERY_PAGES', 5)))
const GALLERY_P95_MS = num('LOAD_GALLERY_P95_MS', 500)
const RANGE_BYTES = Math.max(1, Math.floor(num('LOAD_RANGE_BYTES', 1048576)))
const MAX_VIDEOS = Math.max(1, Math.floor(num('LOAD_RANGE_MAX_VIDEOS', 50)))
const MAX_ERROR_RATE = num('LOAD_MAX_ERROR_RATE', 0.05)
const DISCOVER_PAGES = Math.max(1, Math.floor(num('LOAD_DISCOVER_PAGES', 200)))

const badResponses = new Rate('bad_responses')
const rangeChecks = new Counter('range_checks')

export const options = {
  scenarios: {
    gallery: {
      executor: 'constant-vus',
      exec: 'galleryReader',
      vus: GALLERY_VUS,
      duration: DURATION,
      gracefulStop: '30s'
    },
    ranges: {
      executor: 'constant-vus',
      exec: 'rangeReader',
      vus: RANGE_VUS,
      duration: DURATION,
      gracefulStop: '30s'
    }
  },
  thresholds: {
    'http_req_duration{name:gallery}': [{ threshold: `p(95)<${GALLERY_P95_MS}`, abortOnFail: false }],
    bad_responses: [{ threshold: `rate<${MAX_ERROR_RATE}`, abortOnFail: true, delayAbortEval: '30s' }]
  }
}

function header(res, name) {
  const wanted = name.toLowerCase()
  for (const key of Object.keys(res.headers)) {
    if (key.toLowerCase() === wanted) return res.headers[key]
  }
  return undefined
}

function describe(res) {
  return `${res.status} ${(res.body && String(res.body).slice(0, 120)) || ''}`.trim()
}

export function setup() {
  const health = http.get(`${BASE_URL}/api/health`)
  if (health.status !== 200) fail(`health check against ${BASE_URL} returned ${health.status}`)

  // The guest cookie is Secure: forward it by hand, as the browser would.
  const entry = http.get(`${BASE_URL}/e/${EVENT_TOKEN}`, { redirects: 0 })
  const cookieValue = entry.cookies.guest_session && entry.cookies.guest_session[0] && entry.cookies.guest_session[0].value
  if (entry.status !== 303 || !cookieValue) fail(`guest entry returned ${entry.status} without a guest_session cookie`)
  const cookie = `guest_session=${cookieValue}`

  const videos = []
  let cursor = ''
  for (let page = 0; page < DISCOVER_PAGES && videos.length < MAX_VIDEOS; page++) {
    const url = `${BASE_URL}/api/media?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
    const res = http.get(url, { headers: { Cookie: cookie } })
    if (res.status !== 200) fail(`gallery discovery returned ${describe(res)}`)
    const body = res.json()
    for (const item of body.items) {
      if (videos.length >= MAX_VIDEOS) break
      if (item.mime && item.mime.startsWith('video/') && item.original_url) {
        videos.push({ url: `${BASE_URL}${item.original_url}`, total: 0 })
      }
    }
    cursor = body.next_cursor || ''
    if (!cursor) break
  }
  if (videos.length === 0) {
    fail('no confirmed videos in the gallery: upload some first with scripts/load/slow-upload.sh')
  }

  // Learn each file's size with a one-byte Range probe; every later check uses it.
  const sized = []
  for (const video of videos) {
    const probe = http.get(video.url, { headers: { Cookie: cookie, Range: 'bytes=0-0' }, responseType: 'none' })
    const range = header(probe, 'Content-Range') || ''
    const match = /^bytes 0-0\/(\d+)$/.exec(range)
    if (probe.status === 206 && match) {
      sized.push({ url: video.url, total: Number(match[1]) })
    } else {
      console.warn(`skipping ${video.url}: probe answered ${probe.status} ${range}`)
    }
  }
  if (sized.length === 0) fail('no video answered the Range probe with 206; check the media route')

  console.log(`readers: ${sized.length} videos, ${GALLERY_VUS} gallery VUs, ${RANGE_VUS} range VUs, ${DURATION}, range ${RANGE_BYTES} bytes`)
  return { cookie, videos: sized }
}

export function galleryReader(data) {
  let cursor = ''
  for (let page = 0; page < GALLERY_PAGES; page++) {
    const url = `${BASE_URL}/api/media?limit=${GALLERY_LIMIT}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`
    const res = http.get(url, { headers: { Cookie: data.cookie }, tags: { name: 'gallery' } })
    let body = null
    if (res.status === 200) {
      try {
        body = res.json()
      } catch (_) {
        body = null
      }
    }
    const ok = check(res, { 'gallery 200 with JSON': () => body !== null })
    badResponses.add(!ok)
    if (!ok) {
      console.warn(`gallery page answered ${describe(res)}`)
      break
    }
    cursor = body.next_cursor || ''
    if (!cursor) break
    sleep(0.5 + Math.random() * 1.5)
  }
  sleep(1 + Math.random() * 2)
}

export function rangeReader(data) {
  const video = data.videos[Math.floor(Math.random() * data.videos.length)]
  const length = Math.min(RANGE_BYTES, video.total)
  const start = Math.floor(Math.random() * (video.total - length + 1))
  const end = start + length - 1
  const res = http.get(video.url, {
    headers: { Cookie: data.cookie, Range: `bytes=${start}-${end}` },
    responseType: 'none',
    tags: { name: 'range' }
  })
  rangeChecks.add(1)

  const expectedRange = `bytes ${start}-${end}/${video.total}`
  const contentRange = header(res, 'Content-Range')
  const contentLength = Number(header(res, 'Content-Length'))
  const ok = check(res, {
    'range 206': (r) => r.status === 206,
    'range Content-Range exact': () => contentRange === expectedRange,
    'range Content-Length exact': () => contentLength === length
  })
  badResponses.add(!ok)
  if (!ok) console.warn(`range ${video.url} ${start}-${end}: answered ${res.status}, Content-Range "${contentRange}", Content-Length ${contentLength}`)
  sleep(0.2 + Math.random() * 0.8)
}
