import type { Image } from './images'
import type { ImageGroup } from './imageGroups'
import type { ImageEvent } from './events'
import type { Failure } from './failures'
import type { ProxyMetrics } from './metrics'
import type { Host, HostContainer } from './hosts'

export interface DashboardSection<T> {
  data: T | null
  error: string | null
}

export interface DashboardSummary {
  images: {
    total: number
    registries: number
  }
}

export interface DashboardUpdates {
  images: Image[]
  groups: ImageGroup[]
  total: number
}

export interface DashboardActivity {
  events: ImageEvent[]
  total: number
}

export interface DashboardIssues {
  failures: Failure[]
  distinct: number
  total: number
}

export interface DashboardHosts {
  online: number
  total: number
  outdated: number
  behind: DashboardBehind[]
  trackedContainers: number
  trackedHosts: number
  runningContainers: number
  attention: Host[]
}

export interface DashboardBehind {
  hostId: number
  hostName: string
  container: HostContainer
}

export interface Dashboard {
  summary: DashboardSection<DashboardSummary>
  updates: DashboardSection<DashboardUpdates>
  issues: DashboardSection<DashboardIssues>
  activity: DashboardSection<DashboardActivity>
  metrics: DashboardSection<ProxyMetrics>
  hosts: DashboardSection<DashboardHosts>
}
