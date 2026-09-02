import { get } from 'svelte/store'
import { dashboardStore, initialDashboardState } from '../stores/dashboard'
import { getDashboard } from '../api/dashboard'
import { errorMessage } from '../api/client'
import { createVisibilityPoller } from '../utils/visibilityPoller'
import { tagRefreshStatus } from './tagRefreshStatus'

const POLL_INTERVAL_MS = 60_000

let inflight: Promise<void> | null = null

function stale(): boolean {
  const { settledAt } = get(dashboardStore)

  return settledAt === null || Date.now() - settledAt >= POLL_INTERVAL_MS
}

function loadSections(): Promise<void> {
  return getDashboard()
    .then((data) => {
      dashboardStore.set({ data, loading: false, error: null, settledAt: Date.now() })
    })
    .catch((err) => {
      dashboardStore.update((state) => ({
        ...state,
        loading: false,
        error: errorMessage(err, 'Failed to load the dashboard'),
        settledAt: Date.now(),
      }))
    })
}

function load(): Promise<void> {
  if (inflight) return inflight

  dashboardStore.update((state) => ({ ...state, loading: true }))

  inflight = Promise.all([loadSections(), tagRefreshStatus.refresh()])
    .then(() => undefined)
    .finally(() => {
      inflight = null
    })

  return inflight
}

const poller = createVisibilityPoller({
  tick: load,
  intervalMs: POLL_INTERVAL_MS,
  refetchOnResume: stale,
})

function refresh(): Promise<void> {
  const settled = load()

  poller.reschedule()

  return settled
}

function start(): () => void {
  const stop = poller.start()

  return () => {
    stop()
    dashboardStore.set({ ...initialDashboardState })
  }
}

export const dashboard = {
  subscribe: dashboardStore.subscribe,
  refresh,
  start,
}
