/** The back-to-top button appears once the guest has scrolled down at least one viewport (S14). */
export function shouldShowBackToTop(scrollY: number, viewportHeight: number): boolean {
  return viewportHeight > 0 && scrollY >= viewportHeight
}
