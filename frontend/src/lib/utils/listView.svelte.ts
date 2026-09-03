import { errorMessage } from '../api/client'
import { readListQuery, pushListQuery, watchListQuery } from './listQuery'

type Filters = Record<string, string | number | boolean> & { offset: number }

interface ListViewOptions<F extends Filters, T> {
  routePath: string
  defaults: F
  errorFallback: string
  fetch: (query: F) => Promise<{ items: T[]; total: number }>
}

export function createListView<F extends Filters, T>(options: ListViewOptions<F, T>) {
  const { routePath, defaults, errorFallback, fetch } = options

  const filters = $state<F>({ ...defaults })
  let items = $state<T[]>([])
  let total = $state(0)
  let loading = $state(true)
  let loaded = $state(false)
  let error = $state<string | null>(null)

  let token = 0

  const keys = Object.keys(defaults) as (keyof F)[]
  const filterKeys = keys.filter((k) => k !== 'offset')

  const hasActiveFilters = $derived(filterKeys.some((k) => filters[k] !== defaults[k]))

  async function load() {
    const requestId = ++token
    loading = true
    error = null

    try {
      const result = await fetch({ ...filters })
      if (requestId !== token) return
      items = result.items
      total = result.total
    } catch (err) {
      if (requestId !== token) return
      error = errorMessage(err, errorFallback)
    } finally {
      if (requestId === token) {
        loading = false
        loaded = true
      }
    }
  }

  function syncToUrl() {
    pushListQuery(routePath, { ...filters })
  }

  function syncFromUrl() {
    const parsed = readListQuery(defaults)
    for (const k of keys) filters[k] = parsed[k]
    load()
  }

  return {
    get filters() {
      return filters
    },
    get items() {
      return items
    },
    get total() {
      return total
    },
    get loading() {
      return loading
    },
    get loaded() {
      return loaded
    },
    get error() {
      return error
    },
    get hasActiveFilters() {
      return hasActiveFilters
    },
    init() {
      return watchListQuery(syncFromUrl)
    },
    reload: load,
    setOffset(next: number) {
      filters.offset = next
      syncToUrl()
      load()
    },
    applyFilters() {
      filters.offset = 0
      syncToUrl()
      load()
    },
    clear() {
      for (const k of keys) filters[k] = defaults[k]
      syncToUrl()
      load()
    },
  }
}
