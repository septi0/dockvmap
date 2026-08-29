package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
)

const sessionCleanupInterval = time.Hour
const tagNotificationInterval = 5 * time.Minute
const blobOrphanScanInterval = 24 * time.Hour
const tagDiscoveryCleanupInterval = 24 * time.Hour
const tagDiscoveryRetention = 7 * 24 * time.Hour
const backgroundFailureCleanupInterval = 24 * time.Hour
const backgroundFailureRetention = 30 * 24 * time.Hour
const proxyMetricsFlushInterval = time.Minute
const proxyMetricsCleanupInterval = 24 * time.Hour
const proxyMetricsRetention = 30 * 24 * time.Hour

const workerStartStagger = 10 * time.Second
const maxWorkerStartStagger = 30 * time.Second
const shutdownHookTimeout = 5 * time.Second

type scheduledJob struct {
	name       string
	interval   time.Duration
	run        func(context.Context) error
	onShutdown func(context.Context) error
}

type workerDeps struct {
	cfg                 *config.Config
	schedule            *service.WorkerSchedule
	failures            *service.FailureLog
	images              *service.Images
	discoveries         *service.Discoveries
	cache               *blobcache.Cache
	notifications       *service.Notifications
	sessions            *service.Sessions
	proxyMetrics        *proxy.Metrics
	proxyMetricsHistory *service.ProxyMetricsHistory
}

func startWorker(ctx context.Context, deps workerDeps) {
	cfg := deps.cfg

	jobs := []scheduledJob{
		{
			name:     "session-cleanup",
			interval: sessionCleanupInterval,
			run:      func(ctx context.Context) error { return runSessionCleanup(ctx, deps.sessions) },
		},
		{
			name:     "tag-discovery-cleanup",
			interval: tagDiscoveryCleanupInterval,
			run:      func(ctx context.Context) error { return runTagDiscoveryCleanup(ctx, deps.discoveries) },
		},
		{
			name:     "background-failure-cleanup",
			interval: backgroundFailureCleanupInterval,
			run:      func(ctx context.Context) error { return runBackgroundFailureCleanup(ctx, deps.failures) },
		},
	}

	if interval, err := time.ParseDuration(cfg.TagsCheckInterval); err == nil && interval > 0 {
		jobs = append(jobs, scheduledJob{
			name:     service.WorkerJobTagRefresh,
			interval: interval,
			run:      func(ctx context.Context) error { return runImageTagRefresh(ctx, deps.images) },
		})
	} else {
		slog.Warn("image tag refresh worker disabled", "interval", cfg.TagsCheckInterval)
	}

	if deps.cache != nil {
		if interval, err := time.ParseDuration(cfg.BlobCache.CleanupInterval); err == nil && interval > 0 {
			jobs = append(jobs, scheduledJob{
				name:     "blob-cache-cleanup",
				interval: interval,
				run:      func(ctx context.Context) error { return runBlobCacheCleanup(ctx, deps.cache) },
			})
		} else {
			slog.Warn("blob cache cleanup worker disabled", "interval", cfg.BlobCache.CleanupInterval)
		}

		jobs = append(jobs, scheduledJob{
			name:     "blob-cache-orphan-scan",
			interval: blobOrphanScanInterval,
			run:      func(ctx context.Context) error { return runBlobCacheOrphanScan(ctx, deps.cache) },
		})
	}

	if cfg.SMTP.Enabled || len(cfg.Webhooks) > 0 {
		jobs = append(jobs, scheduledJob{
			name:     "tag-notification",
			interval: tagNotificationInterval,
			run:      func(ctx context.Context) error { return runTagNotification(ctx, deps.notifications) },
		})
	}

	if deps.proxyMetrics != nil && deps.proxyMetricsHistory != nil {
		flusher := &proxyMetricsFlusher{metrics: deps.proxyMetrics, history: deps.proxyMetricsHistory}

		jobs = append(jobs,
			scheduledJob{
				name:       "proxy-metrics-flush",
				interval:   proxyMetricsFlushInterval,
				run:        flusher.flush,
				onShutdown: flusher.flush,
			},
			scheduledJob{
				name:     "proxy-metrics-cleanup",
				interval: proxyMetricsCleanupInterval,
				run:      func(ctx context.Context) error { return runProxyMetricsCleanup(ctx, deps.proxyMetricsHistory) },
			},
		)
	}

	var wg sync.WaitGroup

	for i, job := range jobs {
		offset := min(time.Duration(i)*workerStartStagger, maxWorkerStartStagger)

		slog.Info("starting background worker", "job", job.name, "interval", job.interval, "start_delay", offset)

		wg.Add(1)

		go func() {
			defer wg.Done()
			runScheduledJob(ctx, job, deps.schedule, offset)
		}()
	}

	wg.Wait()
}

func runScheduledJob(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule, offset time.Duration) {
	timer := time.NewTimer(firstRunDelay(ctx, job, schedule, offset))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			start := time.Now()

			executeJob(ctx, job, schedule)

			next := job.interval - time.Since(start)
			if next < 0 {
				next = 0
			}

			timer.Reset(next)

		case <-ctx.Done():
			if job.onShutdown != nil {
				hookCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownHookTimeout)

				if err := job.onShutdown(hookCtx); err != nil {
					slog.Error(job.name+" shutdown hook failed", "error", err)
				}

				cancel()
			}

			slog.Info(job.name + " worker stopped")
			return
		}
	}
}

func firstRunDelay(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule, offset time.Duration) time.Duration {
	lastRun, ok, err := schedule.LastRun(ctx, job.name)

	if err != nil {
		slog.Error("reading worker schedule failed, deferring first run by a full interval", "job", job.name, "error", err)
		return job.interval + offset
	}

	if !ok {
		return offset
	}

	remaining := job.interval - time.Since(lastRun)
	if remaining < 0 {
		remaining = 0
	}

	return remaining + offset
}

func executeJob(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(job.name+" worker panicked", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := schedule.MarkRun(markCtx, job.name); err != nil {
		slog.Error("recording worker run failed", "job", job.name, "error", err)
	}

	if err := job.run(ctx); err != nil {
		slog.Error(job.name+" tick failed", "error", err)
	}
}

func runImageTagRefresh(ctx context.Context, images *service.Images) error {
	slog.Info("refreshing image tags for all configured images")

	start := time.Now()

	refreshed, err := images.RefreshAll(ctx)

	if err != nil {
		return err
	}

	if refreshed > 0 {
		slog.Info("image tag refresh completed", "refreshed", refreshed, "elapsed", time.Since(start))
	}

	return nil
}

func runTagNotification(ctx context.Context, notifications *service.Notifications) error {
	sent, err := notifications.SendPendingTagNotifications(ctx)

	if err != nil {
		return err
	}

	if sent > 0 {
		slog.Info("sent tag notifications", "count", sent)
	}

	return nil
}

func runTagDiscoveryCleanup(ctx context.Context, discoveries *service.Discoveries) error {
	deleted, err := discoveries.CleanupOld(ctx, tagDiscoveryRetention)

	if err != nil {
		return err
	}

	if deleted > 0 {
		slog.Info("removed old tag discoveries", "count", deleted)
	}

	return nil
}

func runBackgroundFailureCleanup(ctx context.Context, failures *service.FailureLog) error {
	deleted, err := failures.CleanupOld(ctx, backgroundFailureRetention)

	if err != nil {
		return err
	}

	if deleted > 0 {
		slog.Info("removed old background failures", "count", deleted)
	}

	return nil
}

func runSessionCleanup(ctx context.Context, sessions *service.Sessions) error {
	deleted, err := sessions.CleanupExpired(ctx)

	if err != nil {
		return err
	}

	if deleted > 0 {
		slog.Info("removed expired sessions", "count", deleted)
	}

	return nil
}

func runBlobCacheCleanup(ctx context.Context, cache *blobcache.Cache) error {
	deleted, err := cache.Cleanup(ctx)

	if err != nil {
		return err
	}

	if deleted > 0 {
		slog.Info("removed expired cached blobs", "count", deleted)
	}

	return nil
}

func runBlobCacheOrphanScan(ctx context.Context, cache *blobcache.Cache) error {
	removed, err := cache.ScanOrphans(ctx)

	if err != nil {
		return err
	}

	if removed > 0 {
		slog.Info("removed orphaned cached blob files", "count", removed)
	}

	return nil
}

func runProxyMetricsCleanup(ctx context.Context, history *service.ProxyMetricsHistory) error {
	deleted, err := history.CleanupOld(ctx, proxyMetricsRetention)

	if err != nil {
		return err
	}

	if deleted > 0 {
		slog.Info("removed old proxy metrics", "count", deleted)
	}

	return nil
}

type proxyMetricsFlusher struct {
	metrics *proxy.Metrics
	history *service.ProxyMetricsHistory
	prev    proxy.MetricsSnapshot
}

func (f *proxyMetricsFlusher) flush(ctx context.Context) error {
	cur := f.metrics.Snapshot()

	delta := model.ProxyMetricsCounters{
		TotalRequests:      int64(cur.TotalRequests - f.prev.TotalRequests),
		ManifestRequests:   int64(cur.ManifestRequests - f.prev.ManifestRequests),
		BlobRequests:       int64(cur.BlobRequests - f.prev.BlobRequests),
		CacheHits:          int64(cur.CacheHits - f.prev.CacheHits),
		CacheMisses:        int64(cur.CacheMisses - f.prev.CacheMisses),
		UpstreamRequests:   int64(cur.UpstreamRequests - f.prev.UpstreamRequests),
		UpstreamFailures:   int64(cur.UpstreamFailures - f.prev.UpstreamFailures),
		CacheWriteFailures: int64(cur.CacheWriteFailures - f.prev.CacheWriteFailures),
	}

	if delta == (model.ProxyMetricsCounters{}) {
		return nil
	}

	if err := f.history.RecordDelta(ctx, delta); err != nil {
		return err
	}

	f.prev = cur

	return nil
}
