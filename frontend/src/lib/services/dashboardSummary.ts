import { dashboardSummaryStore } from '../stores/dashboardSummary'
import { getDashboardSummary } from '../api/dashboard'
import { ApiError } from '../api/client'

let inflight: Promise<boolean> | null = null

function load(): Promise<boolean> {
  if (inflight) return inflight

  dashboardSummaryStore.update((state) => ({ ...state, loading: true, error: null }))

  inflight = getDashboardSummary()
    .then((data) => {
      dashboardSummaryStore.set({ data, loading: false, error: null })
      return true
    })
    .catch((err) => {
      dashboardSummaryStore.update((state) => ({
        ...state,
        loading: false,
        error:
          err instanceof ApiError ? err.message : 'Failed to load dashboard summary',
      }))
      return false
    })
    .finally(() => {
      inflight = null
    })

  return inflight
}

export const dashboardSummary = {
  subscribe: dashboardSummaryStore.subscribe,
  load,
}
