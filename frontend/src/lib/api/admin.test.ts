import { describe, expect, it, vi } from 'vitest'
import {
  AdminSessionRequiredError,
  fetchAdminStatus,
  loginAdmin,
  logoutAdmin,
  parseAdminStatus,
  setUploadsEnabled
} from './admin'

const status = {
  uploadsEnabled: true,
  diskPaused: false,
  freeBytes: 10_000_000_000,
  reservedBytes: 1_000_000_000,
  minFreeBytes: 5_000_000_000
}

function response(init: { status: number; body?: unknown }) {
  return {
    ok: init.status >= 200 && init.status < 300,
    status: init.status,
    json: vi.fn().mockResolvedValue(init.body)
  }
}

describe('admin login', () => {
  it('posts the password as JSON and reports success on 204', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 204 }))

    await expect(loginAdmin('secreto', fetchFn)).resolves.toBe('ok')
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/login',
      expect.objectContaining({
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: 'secreto' })
      })
    )
  })

  it('reports a wrong password without throwing', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'invalid_credentials' } }))

    await expect(loginAdmin('mal', fetchFn)).resolves.toBe('invalid_credentials')
  })

  it('throws for unexpected server errors', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 500 }))

    await expect(loginAdmin('x', fetchFn)).rejects.toThrow(Error)
  })
})

describe('admin status and settings', () => {
  it('reads and validates the status document', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: status }))

    await expect(fetchAdminStatus(fetchFn)).resolves.toEqual(status)
    expect(fetchFn).toHaveBeenCalledWith('/api/admin/status', expect.objectContaining({ credentials: 'same-origin' }))
  })

  it('signals a missing admin session so the view can show the login form', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401, body: { error: 'admin_session_required' } }))

    await expect(fetchAdminStatus(fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })

  it('sends the switch position and returns the refreshed status', async () => {
    const paused = { ...status, uploadsEnabled: false }
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 200, body: paused }))

    await expect(setUploadsEnabled(false, fetchFn)).resolves.toEqual(paused)
    expect(fetchFn).toHaveBeenCalledWith(
      '/api/admin/settings',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ uploadsEnabled: false }) })
    )
  })

  it('rejects status documents with the wrong shape', () => {
    expect(() => parseAdminStatus({ ...status, freeBytes: '10' })).toThrow(Error)
    expect(() => parseAdminStatus({ ...status, uploadsEnabled: 'yes' })).toThrow(Error)
    const { diskPaused: _omitted, ...incomplete } = status
    expect(() => parseAdminStatus(incomplete)).toThrow(Error)
  })

  it('reports a session lost during a settings change as AdminSessionRequiredError', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 401 }))

    await expect(setUploadsEnabled(true, fetchFn)).rejects.toBeInstanceOf(AdminSessionRequiredError)
  })
})

describe('admin logout', () => {
  it('posts logout and resolves on 204', async () => {
    const fetchFn = vi.fn().mockResolvedValue(response({ status: 204 }))

    await expect(logoutAdmin(fetchFn)).resolves.toBeUndefined()
    expect(fetchFn).toHaveBeenCalledWith('/api/admin/logout', expect.objectContaining({ method: 'POST' }))
  })
})
