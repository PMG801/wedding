export async function fetchHealth(fetchFn: typeof fetch = fetch): Promise<{ status: string }> {
  const response = await fetchFn('/api/health')

  if (!response.ok) {
    throw new Error('Health check request failed')
  }

  const body: unknown = await response.json()
  if (
    typeof body !== 'object' ||
    body === null ||
    !('status' in body) ||
    typeof body.status !== 'string'
  ) {
    throw new Error('Health check response has an invalid shape')
  }

  return { status: body.status }
}
