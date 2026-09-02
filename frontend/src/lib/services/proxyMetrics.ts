import { proxyMetricsStore } from '../stores/proxyMetrics'
import { getProxyMetrics } from '../api/metrics'
import { ApiError } from '../api/client'

let inflight: Promise<boolean> | null = null

// Coalesces concurrent callers (the dashboard mounts two consumers at once)
// into a single request; a later mount with no request in flight refetches.
// Resolves to whether the fetch succeeded.
function load(): Promise<boolean> {
  if (inflight) return inflight

  proxyMetricsStore.update((state) => ({ ...state, loading: true, error: null }))

  inflight = getProxyMetrics()
    .then((data) => {
      proxyMetricsStore.set({ data, loading: false, error: null })
      return true
    })
    .catch((err) => {
      proxyMetricsStore.update((state) => ({
        ...state,
        loading: false,
        error: err instanceof ApiError ? err.message : 'Failed to load proxy metrics',
      }))
      return false
    })
    .finally(() => {
      inflight = null
    })

  return inflight
}

export const proxyMetrics = {
  subscribe: proxyMetricsStore.subscribe,
  load,
}
