package proxy

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	startedAt          time.Time
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
	StartedAt          time.Time `json:"startedAt"`
	TotalRequests      uint64    `json:"totalRequests"`
	ManifestRequests   uint64    `json:"manifestRequests"`
	BlobRequests       uint64    `json:"blobRequests"`
	CacheHits          uint64    `json:"cacheHits"`
	CacheMisses        uint64    `json:"cacheMisses"`
	UpstreamRequests   uint64    `json:"upstreamRequests"`
	UpstreamFailures   uint64    `json:"upstreamFailures"`
	CacheWriteFailures uint64    `json:"cacheWriteFailures"`
}

func NewMetrics() *Metrics {
	return &Metrics{startedAt: time.Now().UTC()}
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		StartedAt:          m.startedAt,
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
