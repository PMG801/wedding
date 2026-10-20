import type { AdminStatus } from '../api/admin'
import { adminCopy } from '../i18n/admin'

/** Decimal gigabytes with a Spanish decimal comma, e.g. 12345678901 -> "12,3 GB". */
export function formatGigabytes(bytes: number): string {
  return `${(bytes / 1e9).toFixed(1).replace('.', ',')} GB`
}

/** What the switch shows now, and the value a press will request. */
export function uploadsToggle(status: AdminStatus): { stateLabel: string; actionLabel: string; nextEnabled: boolean } {
  return status.uploadsEnabled
    ? { stateLabel: adminCopy.uploads.open, actionLabel: adminCopy.uploads.pause, nextEnabled: false }
    : { stateLabel: adminCopy.uploads.paused, actionLabel: adminCopy.uploads.resume, nextEnabled: true }
}

/** Automatic disk-pause warning; null while the disk has room. */
export function diskNotice(status: AdminStatus): string | null {
  return status.diskPaused ? adminCopy.disk.autoPaused : null
}

export function canSubmitPassword(password: string): boolean {
  return password.trim() !== ''
}
