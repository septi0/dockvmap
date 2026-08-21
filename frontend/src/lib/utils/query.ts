export function buildQuery<T extends object>(params: T): string {
  const search = new URLSearchParams()

  for (const [key, value] of Object.entries(params) as [string, unknown][]) {
    if (value === undefined || value === null || value === '') continue
    search.set(key, String(value))
  }

  const query = search.toString()

  return query ? `?${query}` : ''
}
