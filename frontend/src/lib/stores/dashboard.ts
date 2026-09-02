import { derived, writable } from 'svelte/store'
import type { Dashboard, DashboardSection } from '../api/types/dashboard'

export interface DashboardState {
  data: Dashboard | null
  loading: boolean
  error: string | null
  settledAt: number | null
}

export const initialDashboardState: DashboardState = {
  data: null,
  loading: false,
  error: null,
  settledAt: null,
}

export const dashboardStore = writable<DashboardState>({ ...initialDashboardState })

export interface DashboardSectionView<T> {
  data: T | null
  error: string | null
  loading: boolean
  busy: boolean
}

export const dashboardSections = derived(dashboardStore, (state) => {
  const loading = state.loading && state.data === null
  const busy = state.loading && state.data !== null

  function view<T>(section?: DashboardSection<T>): DashboardSectionView<T> {
    return {
      data: section?.data ?? null,
      error: state.error ?? section?.error ?? null,
      loading,
      busy,
    }
  }

  return {
    summary: view(state.data?.summary),
    updates: view(state.data?.updates),
    issues: view(state.data?.issues),
    activity: view(state.data?.activity),
    metrics: view(state.data?.metrics),
  }
})
