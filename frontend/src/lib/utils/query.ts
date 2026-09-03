interface BuildQueryOptions {
  dropFalsy?: boolean
}

export function buildQuery<T extends object>(params: T, options: BuildQueryOptions = {}): string {
  const search = new URLSearchParams()

  for (const [key, value] of Object.entries(params) as [string, unknown][]) {
    if (value === undefined || value === null || value === '') continue
    if (options.dropFalsy && (value === false || value === 0)) continue
    search.set(key, String(value))
  }

  const query = search.toString()

  return query ? `?${query}` : ''
}
