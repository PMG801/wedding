/** Sub-pixel rounding can make a fitting body look one pixel taller; that is not an overflow. */
const TOLERANCE_PX = 1

/**
 * True when a clamped pinned body hides text, so the read-more toggle is worth showing (S9).
 * Measured with the DOM's scrollHeight and clientHeight while the body is clamped.
 */
export function bodyOverflows(scrollHeight: number, clientHeight: number): boolean {
  return scrollHeight - clientHeight > TOLERANCE_PX
}
