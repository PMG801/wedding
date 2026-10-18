/** Administrator status as served by GET /api/admin/status and POST /api/admin/settings. */
export type AdminStatus = {
  uploadsEnabled: boolean
  diskPaused: boolean
  freeBytes: number
  reservedBytes: number
  minFreeBytes: number
}

/** Raised when the administrator cookie is missing, tampered with, or expired. */
export class AdminSessionRequiredError extends Error {
  constructor() {
    super('Admin session required')
    this.name = 'AdminSessionRequiredError'
  }
}

const jsonHeaders = { 'Content-Type': 'application/json' }

/** Resolves 'ok' on 204 and 'invalid_credentials' on a wrong password; other failures throw. */
export async function loginAdmin(
  password: string,
  fetchFn: typeof fetch = fetch
): Promise<'ok' | 'invalid_credentials'> {
  const response = await fetchFn('/api/admin/login', {
    method: 'POST',
    credentials: 'same-origin',
    headers: jsonHeaders,
    body: JSON.stringify({ password })
  })
  if (response.status === 204) return 'ok'
  if (response.status === 401) return 'invalid_credentials'
  throw new Error('Admin login request failed')
}

export async function logoutAdmin(fetchFn: typeof fetch = fetch): Promise<void> {
  const response = await fetchFn('/api/admin/logout', { method: 'POST', credentials: 'same-origin' })
  if (!response.ok) throw new Error('Admin logout request failed')
}

export async function fetchAdminStatus(fetchFn: typeof fetch = fetch): Promise<AdminStatus> {
  const response = await fetchFn('/api/admin/status', { credentials: 'same-origin', cache: 'no-store' })
  return readStatus(response)
}

export async function setUploadsEnabled(enabled: boolean, fetchFn: typeof fetch = fetch): Promise<AdminStatus> {
  const response = await fetchFn('/api/admin/settings', {
    method: 'POST',
    credentials: 'same-origin',
    headers: jsonHeaders,
    body: JSON.stringify({ uploadsEnabled: enabled })
  })
  return readStatus(response)
}

async function readStatus(response: Response): Promise<AdminStatus> {
  if (response.status === 401) throw new AdminSessionRequiredError()
  if (!response.ok) throw new Error('Admin status request failed')
  return parseAdminStatus(await response.json())
}

export function parseAdminStatus(body: unknown): AdminStatus {
  if (typeof body !== 'object' || body === null) throw new Error('Admin status has an invalid shape')
  const record = body as Record<string, unknown>
  const isBytes = (value: unknown) => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0
  if (
    typeof record.uploadsEnabled !== 'boolean' ||
    typeof record.diskPaused !== 'boolean' ||
    !isBytes(record.freeBytes) ||
    !isBytes(record.reservedBytes) ||
    !isBytes(record.minFreeBytes)
  ) {
    throw new Error('Admin status has an invalid shape')
  }
  return {
    uploadsEnabled: record.uploadsEnabled,
    diskPaused: record.diskPaused,
    freeBytes: record.freeBytes as number,
    reservedBytes: record.reservedBytes as number,
    minFreeBytes: record.minFreeBytes as number
  }
}
