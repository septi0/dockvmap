package model

type ProxyMetricsCounters struct {
	TotalRequests      int64 `json:"totalRequests"`
	ManifestRequests   int64 `json:"manifestRequests"`
	BlobRequests       int64 `json:"blobRequests"`
	CacheHits          int64 `json:"cacheHits"`
	CacheMisses        int64 `json:"cacheMisses"`
	UpstreamRequests   int64 `json:"upstreamRequests"`
	UpstreamFailures   int64 `json:"upstreamFailures"`
	CacheWriteFailures int64 `json:"cacheWriteFailures"`
}

type ProxyMetricsDay struct {
	Day string `json:"day"`
	ProxyMetricsCounters
}

type ProxyMetricsSummary struct {
	Today      ProxyMetricsCounters `json:"today"`
	Last7Days  ProxyMetricsCounters `json:"last7d"`
	Last30Days ProxyMetricsCounters `json:"last30d"`
}
