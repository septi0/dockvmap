import type { Image } from './images'
import type { ImageEvent } from './events'
import type { RecentFailure } from './failures'
import type { ProxyMetrics } from './metrics'

export interface DashboardSection<T> {
  data: T | null
  error: string | null
}

export interface DashboardSummary {
  images: {
    total: number
    updateAvailable: number
    failedCheck: number
  }
}

export interface DashboardUpdates {
  images: Image[]
  total: number
}

export interface Dashboard {
  summary: DashboardSection<DashboardSummary>
  updates: DashboardSection<DashboardUpdates>
  issues: DashboardSection<RecentFailure[]>
  activity: DashboardSection<ImageEvent[]>
  metrics: DashboardSection<ProxyMetrics>
}
