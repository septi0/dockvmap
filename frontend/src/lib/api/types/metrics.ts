export interface PullInfo {
  host: string
  port: string
  virtualTag: string
  hostConfigured: boolean
}

export interface ProxyMetricsCounters {
  totalRequests: number
  manifestRequests: number
  blobRequests: number
  cacheHits: number
  cacheMisses: number
  upstreamRequests: number
  upstreamFailures: number
  cacheWriteFailures: number
}

export type ProxyMetricsWindow = 'today' | 'last7d' | 'last30d'

export interface ProxyMetrics {
  generatedAt: string
  windows: Record<ProxyMetricsWindow, ProxyMetricsCounters>
  cache: { usedBytes: number; maxBytes: number } | null
}
