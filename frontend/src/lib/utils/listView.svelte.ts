import { errorMessage } from '../api/client'
import type { SortDirection, SortParams } from '../api/types/sort'
import { readListQuery, pushListQuery, watchListQuery } from './listQuery'

type Filters = Record<string, string | number | boolean> & { offset: number }

export const NO_SORT = { sort: '', order: '' }

const SORT_KEYS = Object.keys(NO_SORT)

interface ListViewOptions<F extends Filters, T> {
  routePath: string
  defaults: F
  errorFallback: string
  fetch: (query: F) => Promise<{ items: T[]; total: number }>
  sortFields?: Record<string, SortDirection>
}

export function sortParams<S extends string>(query: Filters): SortParams<S> {
  return query.sort ? { sort: query.sort as S, order: query.order as SortDirection } : {}
}

export function createListView<F extends Filters, T>(options: ListViewOptions<F, T>) {
  const { routePath, defaults, errorFallback, fetch, sortFields = {} } = options

  const filters = $state<F>({ ...defaults })
  let items = $state<T[]>([])
  let total = $state(0)
  let loading = $state(true)
  let loaded = $state(false)
  let error = $state<string | null>(null)

  let token = 0

  const keys = Object.keys(defaults) as (keyof F)[]
  const filterKeys = keys.filter((k) => k !== 'offset' && !SORT_KEYS.includes(k as string))
  const query = filters as Filters

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

    const validSort =
      Object.hasOwn(sortFields, String(query.sort)) && (query.order === 'asc' || query.order === 'desc')

    if ('sort' in query && !validSort) Object.assign(query, NO_SORT)

    load()
  }

  function sortDirection(field: string): SortDirection | null {
    return query.sort === field ? (query.order as SortDirection) : null
  }

  function toggleSort(field: string) {
    const first = sortFields[field]

    if (!first) return

    if (query.sort !== field) {
      Object.assign(query, { sort: field, order: first })
    } else if (query.order === first) {
      query.order = first === 'asc' ? 'desc' : 'asc'
    } else {
      Object.assign(query, NO_SORT)
    }

    query.offset = 0
    syncToUrl()
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
    sortDirection,
    toggleSort,
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
      for (const k of keys) if (!SORT_KEYS.includes(k as string)) filters[k] = defaults[k]
      syncToUrl()
      load()
    },
  }
}
