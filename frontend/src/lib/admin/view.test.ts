import { describe, expect, it } from 'vitest'
import { adminCopy } from '../i18n/admin'
import type { AdminStatus } from '../api/admin'
import { canSubmitPassword, diskNotice, formatGigabytes, isAdminPath, uploadsToggle } from './view'

const enabled: AdminStatus = {
  uploadsEnabled: true,
  diskPaused: false,
  freeBytes: 12_300_000_000,
  reservedBytes: 1_500_000_000,
  minFreeBytes: 5_000_000_000
}

describe('formatGigabytes', () => {
  it('formats decimal gigabytes with a Spanish decimal comma', () => {
    expect(formatGigabytes(0)).toBe('0,0 GB')
    expect(formatGigabytes(1_500_000_000)).toBe('1,5 GB')
    expect(formatGigabytes(12_345_678_901)).toBe('12,3 GB')
  })
})

describe('uploadsToggle', () => {
  it('offers to pause while uploads are open and says so', () => {
    expect(uploadsToggle(enabled)).toEqual({
      stateLabel: adminCopy.uploads.open,
      actionLabel: adminCopy.uploads.pause,
      nextEnabled: false
    })
  })

  it('offers to resume while uploads are paused and says so', () => {
    expect(uploadsToggle({ ...enabled, uploadsEnabled: false })).toEqual({
      stateLabel: adminCopy.uploads.paused,
      actionLabel: adminCopy.uploads.resume,
      nextEnabled: true
    })
  })
})

describe('diskNotice', () => {
  it('stays silent while the disk has room', () => {
    expect(diskNotice(enabled)).toBeNull()
  })

  it('explains the automatic pause when free space is under the reserve', () => {
    const notice = diskNotice({ ...enabled, diskPaused: true })
    expect(notice).toBe(adminCopy.disk.autoPaused)
    expect(notice).toContain('espacio')
  })
})

describe('canSubmitPassword', () => {
  it('only enables the login button for a non-empty password', () => {
    expect(canSubmitPassword('')).toBe(false)
    expect(canSubmitPassword('   ')).toBe(false)
    expect(canSubmitPassword('secreto')).toBe(true)
  })
})

describe('isAdminPath', () => {
  it('routes only the admin page to the administrator view', () => {
    expect(isAdminPath('/admin')).toBe(true)
    expect(isAdminPath('/admin/')).toBe(true)
    expect(isAdminPath('/')).toBe(false)
    expect(isAdminPath('/admin/extra')).toBe(false)
    expect(isAdminPath('/administrator')).toBe(false)
  })
})
