import { api } from './client'
import type { PullInfo, ProxyMetrics } from './types/metrics'

export function getPullInfo() {
  return api.get<PullInfo>('/proxy/pull-info')
}

export function getProxyMetrics() {
  return api.get<ProxyMetrics>('/proxy-metrics')
}
