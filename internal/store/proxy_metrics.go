package store

import (
	"context"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) AddProxyMetricsDaily(ctx context.Context, day string, delta model.ProxyMetricsCounters) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proxy_metrics_daily (
			day, total_requests, manifest_requests, blob_requests,
			cache_hits, cache_misses, upstream_requests, upstream_failures, cache_write_failures
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(day) DO UPDATE SET
			total_requests       = total_requests + excluded.total_requests,
			manifest_requests    = manifest_requests + excluded.manifest_requests,
			blob_requests        = blob_requests + excluded.blob_requests,
			cache_hits           = cache_hits + excluded.cache_hits,
			cache_misses         = cache_misses + excluded.cache_misses,
			upstream_requests    = upstream_requests + excluded.upstream_requests,
			upstream_failures    = upstream_failures + excluded.upstream_failures,
			cache_write_failures = cache_write_failures + excluded.cache_write_failures
	`,
		day,
		delta.TotalRequests, delta.ManifestRequests, delta.BlobRequests,
		delta.CacheHits, delta.CacheMisses, delta.UpstreamRequests,
		delta.UpstreamFailures, delta.CacheWriteFailures,
	)

	if err != nil {
		return fmt.Errorf("recording proxy metrics for %s: %w", day, err)
	}

	return nil
}

func (s *Store) ListProxyMetricsDaily(ctx context.Context, sinceDay string) ([]model.ProxyMetricsDay, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, total_requests, manifest_requests, blob_requests,
		       cache_hits, cache_misses, upstream_requests, upstream_failures, cache_write_failures
		FROM proxy_metrics_daily
		WHERE day >= ?
		ORDER BY day ASC
	`, sinceDay)

	if err != nil {
		return nil, fmt.Errorf("listing proxy metrics: %w", err)
	}

	defer rows.Close()

	days := make([]model.ProxyMetricsDay, 0)

	for rows.Next() {
		var d model.ProxyMetricsDay

		if err := rows.Scan(
			&d.Day, &d.TotalRequests, &d.ManifestRequests, &d.BlobRequests,
			&d.CacheHits, &d.CacheMisses, &d.UpstreamRequests, &d.UpstreamFailures, &d.CacheWriteFailures,
		); err != nil {
			return nil, fmt.Errorf("scanning proxy metrics row: %w", err)
		}

		days = append(days, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing proxy metrics: %w", err)
	}

	return days, nil
}

func (s *Store) DeleteProxyMetricsDailyBefore(ctx context.Context, day string) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM proxy_metrics_daily WHERE day < ?", day)

	if err != nil {
		return 0, fmt.Errorf("deleting old proxy metrics: %w", err)
	}

	return result.RowsAffected()
}
