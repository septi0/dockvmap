import { writable } from 'svelte/store'
import type { DashboardSummary } from '../api/types/dashboard'

export interface DashboardSummaryState {
  data: DashboardSummary | null
  loading: boolean
  error: string | null
}

export const dashboardSummaryStore = writable<DashboardSummaryState>({
  data: null,
  loading: false,
  error: null,
})
