// k6 load test for the guest upload flow. Payloads are synthetic (a JPEG
// signature followed by zeros), so no image files are needed: the backend only
// validates the magic bytes and the declared size.
//
// Each photo goes through the tus endpoint (ADR-0014): a POST declares the size and
// one PATCH carries every byte. The idempotent retry repeats the POST, which answers
// 409 already_confirmed.
//
// Every limit is an environment variable so the test stops where YOU decide,
// not where the machine gives up. See scripts/load.sh for the full list.
import http from 'k6/http'
import encoding from 'k6/encoding'
import exec from 'k6/execution'
import { Counter, Rate } from 'k6/metrics'
import { check, fail } from 'k6'
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js'

const num = (name, fallback) => {
  const raw = __ENV[name]
  if (raw === undefined || raw === '') return fallback
  const value = Number(raw)
  if (!Number.isFinite(value) || value < 0) fail(`${name} must be a non-negative number, got "${raw}"`)
  return value
}
const flag = (name, fallback) => (__ENV[name] === undefined ? fallback : ['1', 'true', 'yes'].includes(__ENV[name]))

const BASE_URL = (__ENV.LOAD_BASE_URL || '').replace(/\/$/, '')
const EVENT_TOKEN = __ENV.LOAD_EVENT_TOKEN || ''
if (!BASE_URL || !EVENT_TOKEN) fail('LOAD_BASE_URL and LOAD_EVENT_TOKEN are required')

const PHOTO_BYTES = Math.max(16, Math.floor(num('LOAD_PHOTO_BYTES', 2 * 1024 * 1024)))
const MAX_TOTAL_BYTES = num('LOAD_MAX_TOTAL_BYTES', 100 * 1024 * 1024)
const VUS = Math.max(1, Math.floor(num('LOAD_VUS', 5)))
const MAX_DURATION = __ENV.LOAD_MAX_DURATION || '5m'
const RETRY = flag('LOAD_IDEMPOTENT_RETRY', true)
const GALLERY_READS = Math.floor(num('LOAD_GALLERY_READS', 1))
const MAX_ERROR_RATE = num('LOAD_MAX_ERROR_RATE', 0.05)
const STOP_ON_STATUS = (__ENV.LOAD_STOP_ON_STATUS === undefined ? '507' : __ENV.LOAD_STOP_ON_STATUS)
  .split(',')
  .map((s) => Number(s.trim()))
  .filter((n) => n > 0)

// Hard budget: each iteration sends PHOTO_BYTES (twice when the retry is on),
// so the iteration count is derived from the byte budget and can never exceed it.
const BYTES_PER_ITERATION = PHOTO_BYTES * (RETRY ? 2 : 1)
const ITERATIONS = Math.floor(MAX_TOTAL_BYTES / BYTES_PER_ITERATION)
if (ITERATIONS < 1) fail(`LOAD_MAX_TOTAL_BYTES (${MAX_TOTAL_BYTES}) is smaller than one iteration (${BYTES_PER_ITERATION} bytes)`)

// Statuses that mean "the server enforced one of its configured limits". They
// are reported separately and are not failures.
const LIMIT_STATUSES = [408, 413, 503, 507]
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 399 }, 409, ...LIMIT_STATUSES))

const limitHits = new Counter('limit_hits')
const bytesAccepted = new Counter('bytes_accepted')
const unexpectedErrors = new Rate('unexpected_errors')

export const options = {
  scenarios: {
    guests: {
      executor: 'shared-iterations',
      vus: Math.min(VUS, ITERATIONS),
      iterations: ITERATIONS,
      maxDuration: MAX_DURATION
    }
  },
  thresholds: {
    unexpected_errors: [{ threshold: `rate<${MAX_ERROR_RATE}`, abortOnFail: true, delayAbortEval: '5s' }]
  }
}

export function setup() {
  console.log(
    `plan: ${ITERATIONS} iterations x ${BYTES_PER_ITERATION} bytes = ${ITERATIONS * BYTES_PER_ITERATION} bytes max, ` +
      `${Math.min(VUS, ITERATIONS)} VUs, maxDuration ${MAX_DURATION}, stop on status [${STOP_ON_STATUS}]`
  )
  const health = http.get(`${BASE_URL}/api/health`)
  if (health.status !== 200) fail(`health check against ${BASE_URL} returned ${health.status}`)
}

let cookie = null
let payload = null

function enter() {
  // The guest cookie is Secure, so k6's jar would not replay it over plain HTTP: forward it by hand.
  const res = http.get(`${BASE_URL}/e/${EVENT_TOKEN}`, { redirects: 0 })
  const value = res.cookies.guest_session && res.cookies.guest_session[0] && res.cookies.guest_session[0].value
  if (res.status !== 303 || !value) fail(`guest entry returned ${res.status} without a guest_session cookie`)
  return `guest_session=${value}`
}

// record counts a response and reports whether it is one the step expects.
function record(res, label, expected = (status) => status >= 200 && status < 300) {
  if (LIMIT_STATUSES.includes(res.status)) {
    limitHits.add(1, { status: String(res.status), step: label })
    unexpectedErrors.add(false)
    if (STOP_ON_STATUS.includes(res.status)) {
      exec.test.abort(`server limit reached: ${label} answered ${res.status} (LOAD_STOP_ON_STATUS)`)
    }
    return false
  }
  const ok = expected(res.status)
  unexpectedErrors.add(!ok)
  return ok
}

export default function () {
  if (!cookie) cookie = enter()
  if (!payload) {
    const bytes = new Uint8Array(PHOTO_BYTES)
    bytes.set([0xff, 0xd8, 0xff, 0xe0])
    payload = bytes.buffer
  }
  const id = uuidv4()
  const headers = { Cookie: cookie, 'Tus-Resumable': '1.0.0' }
  const metadata = `id ${encoding.b64encode(id)},filetype ${encoding.b64encode('image/jpeg')}`
  const create = (step) =>
    http.post(`${BASE_URL}/api/uploads/`, null, {
      headers: { ...headers, 'Upload-Length': String(PHOTO_BYTES), 'Upload-Metadata': metadata },
      tags: { step }
    })

  const created = create('upload')
  if (record(created, 'upload', (status) => status === 201)) {
    const patch = http.patch(`${BASE_URL}/api/uploads/${id}`, payload, {
      headers: { ...headers, 'Upload-Offset': '0', 'Content-Type': 'application/offset+octet-stream' },
      tags: { step: 'upload' }
    })
    if (record(patch, 'upload', (status) => status === 204)) {
      bytesAccepted.add(PHOTO_BYTES)
      check(patch, { 'upload completed': (r) => r.status === 204 })

      if (RETRY) {
        const retry = create('retry')
        if (record(retry, 'retry', (status) => status === 409)) {
          check(retry, { 'retry reports already confirmed': (r) => r.status === 409 })
        }
      }
    }
  }

  for (let i = 0; i < GALLERY_READS; i++) {
    const gallery = http.get(`${BASE_URL}/api/media`, { headers: { Cookie: cookie }, tags: { step: 'gallery' } })
    record(gallery, 'gallery')
  }
}
