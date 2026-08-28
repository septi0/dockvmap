type Filters = Record<string, string | number | boolean>

export function readListQuery<T extends Filters>(defaults: T): T {
  const raw = window.location.hash.split('?')[1] ?? ''
  const params = new URLSearchParams(raw)
  const result: Filters = { ...defaults }

  for (const [key, fallback] of Object.entries(defaults)) {
    const value = params.get(key)
    if (value === null) continue

    if (typeof fallback === 'boolean') {
      result[key] = value === 'true'
    } else if (typeof fallback === 'number') {
      const parsed = Number(value)
      result[key] = Number.isFinite(parsed) && parsed >= 0 ? parsed : fallback
    } else {
      result[key] = value
    }
  }

  return result as T
}

export function writeListQuery(filters: Filters): string {
  const params = new URLSearchParams()

  for (const [key, value] of Object.entries(filters)) {
    if (value === '' || value === false || value === 0 || value == null) continue
    params.set(key, String(value))
  }

  const query = params.toString()

  return query ? `?${query}` : ''
}

export function pushListQuery(routePath: string, filters: Filters): void {
  history.replaceState(history.state, '', '#' + routePath + writeListQuery(filters))
}

export function watchListQuery(sync: () => void): () => void {
  sync()
  window.addEventListener('hashchange', sync)
  return () => window.removeEventListener('hashchange', sync)
}
