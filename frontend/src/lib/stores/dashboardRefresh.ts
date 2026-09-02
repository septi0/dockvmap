import { writable } from 'svelte/store'

export interface DashboardRefreshState {
  /** incremented on every manual refresh; cards refetch when it changes */
  nonce: number
  /** ids of cards with a fetch in flight (initial load or a refresh) */
  pending: string[]
  /** ids of cards whose most recent load failed (cleared on a later success) */
  errored: string[]
  /** epoch ms when the last load settled with nothing left pending */
  settledAt: number | null
}

export const dashboardRefreshStore = writable<DashboardRefreshState>({
  nonce: 0,
  pending: [],
  errored: [],
  settledAt: null,
})
