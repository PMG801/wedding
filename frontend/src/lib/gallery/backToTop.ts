/** Show the button around the point where the first photo row leaves view. */
export function shouldShowBackToTop(scrollY: number, viewportHeight: number): boolean {
  return viewportHeight > 0 && scrollY >= viewportHeight * 0.5
}
