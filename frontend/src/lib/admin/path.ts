// Kept apart from view.ts so the guest bundle does not pull in the admin copy (S11).
/** True for /admin with or without a trailing slash; every other path is the guest page. */
export function isAdminPath(pathname: string): boolean {
  return pathname === '/admin' || pathname === '/admin/'
}
