import { writable } from 'svelte/store'
import type { ProxyMetrics } from '../api/types/metrics'

export interface ProxyMetricsState {
  data: ProxyMetrics | null
  loading: boolean
  error: string | null
}

export const proxyMetricsStore = writable<ProxyMetricsState>({
  data: null,
  loading: false,
  error: null,
})
