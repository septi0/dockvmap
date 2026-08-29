package service

import (
	"context"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type proxyMetricsStore interface {
	AddProxyMetricsDaily(ctx context.Context, day string, delta model.ProxyMetricsCounters) error
	ListProxyMetricsDaily(ctx context.Context, sinceDay string) ([]model.ProxyMetricsDay, error)
	DeleteProxyMetricsDailyBefore(ctx context.Context, day string) (int64, error)
}

type ProxyMetricsHistory struct {
	store proxyMetricsStore
}

func NewProxyMetricsHistory(store proxyMetricsStore) *ProxyMetricsHistory {
	return &ProxyMetricsHistory{store: store}
}

func metricsDay(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func (h *ProxyMetricsHistory) RecordDelta(ctx context.Context, delta model.ProxyMetricsCounters) error {
	return h.store.AddProxyMetricsDaily(ctx, metricsDay(time.Now()), delta)
}

func (h *ProxyMetricsHistory) Summary(ctx context.Context) (model.ProxyMetricsSummary, error) {
	now := time.Now().UTC()

	days, err := h.store.ListProxyMetricsDaily(ctx, metricsDay(now.AddDate(0, 0, -29)))

	if err != nil {
		return model.ProxyMetricsSummary{}, err
	}

	today := metricsDay(now)
	weekStart := metricsDay(now.AddDate(0, 0, -6))

	var summary model.ProxyMetricsSummary

	for _, d := range days {
		addCounters(&summary.Last30Days, d.ProxyMetricsCounters)

		if d.Day >= weekStart {
			addCounters(&summary.Last7Days, d.ProxyMetricsCounters)
		}

		if d.Day == today {
			addCounters(&summary.Today, d.ProxyMetricsCounters)
		}
	}

	return summary, nil
}

func (h *ProxyMetricsHistory) CleanupOld(ctx context.Context, retention time.Duration) (int64, error) {
	return h.store.DeleteProxyMetricsDailyBefore(ctx, metricsDay(time.Now().Add(-retention)))
}

func addCounters(dst *model.ProxyMetricsCounters, src model.ProxyMetricsCounters) {
	dst.TotalRequests += src.TotalRequests
	dst.ManifestRequests += src.ManifestRequests
	dst.BlobRequests += src.BlobRequests
	dst.CacheHits += src.CacheHits
	dst.CacheMisses += src.CacheMisses
	dst.UpstreamRequests += src.UpstreamRequests
	dst.UpstreamFailures += src.UpstreamFailures
	dst.CacheWriteFailures += src.CacheWriteFailures
}
