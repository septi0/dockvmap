package proxy

import "sync/atomic"

type Metrics struct {
	totalRequests      atomic.Uint64
	manifestRequests   atomic.Uint64
	blobRequests       atomic.Uint64
	cacheHits          atomic.Uint64
	cacheMisses        atomic.Uint64
	upstreamRequests   atomic.Uint64
	upstreamFailures   atomic.Uint64
	cacheWriteFailures atomic.Uint64
}

type MetricsSnapshot struct {
	TotalRequests      uint64
	ManifestRequests   uint64
	BlobRequests       uint64
	CacheHits          uint64
	CacheMisses        uint64
	UpstreamRequests   uint64
	UpstreamFailures   uint64
	CacheWriteFailures uint64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalRequests:      m.totalRequests.Load(),
		ManifestRequests:   m.manifestRequests.Load(),
		BlobRequests:       m.blobRequests.Load(),
		CacheHits:          m.cacheHits.Load(),
		CacheMisses:        m.cacheMisses.Load(),
		UpstreamRequests:   m.upstreamRequests.Load(),
		UpstreamFailures:   m.upstreamFailures.Load(),
		CacheWriteFailures: m.cacheWriteFailures.Load(),
	}
}
