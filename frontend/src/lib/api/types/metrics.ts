export interface PullInfo {
  host: string
  port: string
  virtualTag: string
  hostConfigured: boolean
}

export interface ProxyMetrics {
  startedAt: string
  cacheEnabled: boolean
  totalRequests: number
  manifestRequests: number
  blobRequests: number
  cacheHits: number
  cacheMisses: number
  upstreamRequests: number
  upstreamFailures: number
  cacheWriteFailures: number
}
