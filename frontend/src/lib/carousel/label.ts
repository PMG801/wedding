/** Time range of a period as shown on its bubble, «HH:MM–HH:MM», or null when either bound is not a valid time. */
export function periodLabel(start: string, end: string): string | null {
  const from = clockOf(start)
  const to = clockOf(end)
  return from === null || to === null ? null : `${from}–${to}`
}

function clockOf(value: string): string | null {
  const match = /T(\d{2}):(\d{2})(?::\d{2})?$/.exec(value)
  if (match === null) return null
  if (Number(match[1]) > 23 || Number(match[2]) > 59) return null
  return `${match[1]}:${match[2]}`
}
