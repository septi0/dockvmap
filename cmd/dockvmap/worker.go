package main

import (
	"context"
	"fmt"
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
const proxyMetricsFlushInterval = 5 * time.Minute
const proxyMetricsCleanupInterval = 24 * time.Hour
const proxyMetricsRetention = 30 * 24 * time.Hour

const workerStartStagger = 10 * time.Second
const maxWorkerStartStagger = 30 * time.Second
const shutdownHookTimeout = 5 * time.Second

type scheduledJob struct {
	name           string
	description    string
	interval       time.Duration
	enabled        bool
	disabledReason string
	run            func(context.Context) (int, error)
	doneMsg        string // logged with "count" when run reports count > 0
	onShutdown     func(context.Context) error
	trigger        <-chan struct{} // nil unless the job can be run early on demand
}

func counted(n int64, err error) (int, error) { return int(n), err }

type workerDeps struct {
	cfg                 *config.Config
	worker              *service.Worker
	failures            *service.FailureLog
	images              *service.Images
	discoveries         *service.Discoveries
	cache               *blobcache.Cache
	notifications       *service.Notifications
	sessions            *service.Sessions
	proxyMetrics        *proxy.Metrics
	proxyMetricsHistory *service.ProxyMetricsHistory
}

func scheduledJobs(deps workerDeps) []scheduledJob {
	cfg := deps.cfg

	jobs := []scheduledJob{
		{
			name:        "session-cleanup",
			description: "Delete expired login sessions.",
			interval:    sessionCleanupInterval,
			enabled:     true,
			doneMsg:     "removed expired sessions",
			run:         func(ctx context.Context) (int, error) { return counted(deps.sessions.CleanupExpired(ctx)) },
		},
		{
			name:        "tag-discovery-cleanup",
			description: "Drop finished tag-discovery records older than 7 days.",
			interval:    tagDiscoveryCleanupInterval,
			enabled:     true,
			doneMsg:     "removed old tag discoveries",
			run: func(ctx context.Context) (int, error) {
				return counted(deps.discoveries.CleanupOld(ctx, tagDiscoveryRetention))
			},
		},
		{
			name:        "background-failure-cleanup",
			description: "Drop background-failure records older than 30 days.",
			interval:    backgroundFailureCleanupInterval,
			enabled:     true,
			doneMsg:     "removed old background failures",
			run: func(ctx context.Context) (int, error) {
				return counted(deps.failures.CleanupOld(ctx, backgroundFailureRetention))
			},
		},
	}

	const tagRefreshDesc = "Poll every tracked image's upstream registry for tag changes."

	if interval := cfg.TagsCheckIntervalDuration(); interval > 0 {
		jobs = append(jobs, scheduledJob{
			name:        service.WorkerJobTagRefresh,
			description: tagRefreshDesc,
			interval:    interval,
			enabled:     true,
			trigger:     deps.worker.Register(service.WorkerJobTagRefresh),
			run:         func(ctx context.Context) (int, error) { return runImageTagRefresh(ctx, deps.images) },
		})
	} else {
		jobs = append(jobs, scheduledJob{
			name:           service.WorkerJobTagRefresh,
			description:    tagRefreshDesc,
			enabled:        false,
			disabledReason: "tags_check_interval is not a positive duration",
		})
	}

	jobs = append(jobs, blobCacheJobs(deps)...)

	const tagNotificationDesc = "Send pending email and webhook notifications for tag changes."

	if cfg.SMTP.Enabled || len(cfg.Webhooks) > 0 {
		jobs = append(jobs, scheduledJob{
			name:        "tag-notification",
			description: tagNotificationDesc,
			interval:    tagNotificationInterval,
			enabled:     true,
			doneMsg:     "sent tag notifications",
			run:         func(ctx context.Context) (int, error) { return deps.notifications.SendPendingTagNotifications(ctx) },
		})
	} else {
		jobs = append(jobs, scheduledJob{
			name:           "tag-notification",
			description:    tagNotificationDesc,
			enabled:        false,
			disabledReason: "no SMTP host and no webhooks are configured",
		})
	}

	jobs = append(jobs, proxyMetricsJobs(deps)...)

	return jobs
}

func blobCacheJobs(deps workerDeps) []scheduledJob {
	const cleanupDesc = "Evict expired cached blobs and enforce the cache size limit."
	const orphanDesc = "Delete cache files that have no database record."

	if deps.cache == nil {
		return []scheduledJob{
			{name: "blob-cache-cleanup", description: cleanupDesc, enabled: false, disabledReason: "the blob cache is disabled"},
			{name: "blob-cache-orphan-scan", description: orphanDesc, enabled: false, disabledReason: "the blob cache is disabled"},
		}
	}

	jobs := make([]scheduledJob, 0, 2)

	if interval, err := time.ParseDuration(deps.cfg.BlobCache.CleanupInterval); err == nil && interval > 0 {
		jobs = append(jobs, scheduledJob{
			name:        "blob-cache-cleanup",
			description: cleanupDesc,
			interval:    interval,
			enabled:     true,
			trigger:     deps.worker.Register("blob-cache-cleanup"),
			doneMsg:     "removed expired cached blobs",
			run:         func(ctx context.Context) (int, error) { return deps.cache.Cleanup(ctx) },
		})
	} else {
		jobs = append(jobs, scheduledJob{
			name:           "blob-cache-cleanup",
			description:    cleanupDesc,
			enabled:        false,
			disabledReason: "blob_cache.cleanup_interval is not a positive duration",
		})
	}

	jobs = append(jobs, scheduledJob{
		name:        "blob-cache-orphan-scan",
		description: orphanDesc,
		interval:    blobOrphanScanInterval,
		enabled:     true,
		trigger:     deps.worker.Register("blob-cache-orphan-scan"),
		doneMsg:     "removed orphaned cached blob files",
		run:         func(ctx context.Context) (int, error) { return deps.cache.ScanOrphans(ctx) },
	})

	return jobs
}

func proxyMetricsJobs(deps workerDeps) []scheduledJob {
	const flushDesc = "Fold in-memory proxy counters into today's stored totals."
	const cleanupDesc = "Drop stored proxy metrics older than 30 days."

	if deps.proxyMetrics == nil || deps.proxyMetricsHistory == nil {
		return []scheduledJob{
			{name: "proxy-metrics-flush", description: flushDesc, enabled: false, disabledReason: "proxy metrics are not available"},
			{name: "proxy-metrics-cleanup", description: cleanupDesc, enabled: false, disabledReason: "proxy metrics are not available"},
		}
	}

	flusher := &proxyMetricsFlusher{metrics: deps.proxyMetrics, history: deps.proxyMetricsHistory}

	return []scheduledJob{
		{
			name:        "proxy-metrics-flush",
			description: flushDesc,
			interval:    proxyMetricsFlushInterval,
			enabled:     true,
			run:         func(ctx context.Context) (int, error) { return 0, flusher.flush(ctx) },
			onShutdown:  flusher.flush,
		},
		{
			name:        "proxy-metrics-cleanup",
			description: cleanupDesc,
			interval:    proxyMetricsCleanupInterval,
			enabled:     true,
			doneMsg:     "removed old proxy metrics",
			run: func(ctx context.Context) (int, error) {
				return counted(deps.proxyMetricsHistory.CleanupOld(ctx, proxyMetricsRetention))
			},
		},
	}
}

func jobDescriptors(jobs []scheduledJob) []service.WorkerJobDescriptor {
	descriptors := make([]service.WorkerJobDescriptor, len(jobs))

	for i, job := range jobs {
		descriptors[i] = service.WorkerJobDescriptor{
			Name:           job.name,
			Description:    job.description,
			Interval:       job.interval,
			Enabled:        job.enabled,
			DisabledReason: job.disabledReason,
			Triggerable:    job.trigger != nil,
		}
	}

	return descriptors
}

func runScheduledJobs(ctx context.Context, jobs []scheduledJob, worker *service.Worker) {
	var wg sync.WaitGroup

	staggerIndex := 0

	for _, job := range jobs {
		if !job.enabled {
			slog.Warn("background worker disabled", "job", job.name, "reason", job.disabledReason)
			continue
		}

		offset := min(time.Duration(staggerIndex)*workerStartStagger, maxWorkerStartStagger)
		staggerIndex++

		slog.Info("starting background worker", "job", job.name, "interval", job.interval, "start_delay", offset)

		wg.Add(1)

		go func() {
			defer wg.Done()
			runScheduledJob(ctx, job, worker, offset)
		}()
	}

	wg.Wait()
}

func runScheduledJob(ctx context.Context, job scheduledJob, worker *service.Worker, offset time.Duration) {
	timer := time.NewTimer(firstRunDelay(ctx, job, worker, offset))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			runAndReschedule(ctx, job, worker, timer)

		case <-job.trigger:
			runAndReschedule(ctx, job, worker, timer)

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

// a manual trigger reschedules a full interval from now, exactly like a natural tick
func runAndReschedule(ctx context.Context, job scheduledJob, worker *service.Worker, timer *time.Timer) {
	start := time.Now()

	executeJob(ctx, job, worker)

	next := rescheduleDelay(job.interval, time.Since(start))

	// via job.trigger the timer is still armed; stop and drain before Reset so a stale fire can't leak through
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}

	timer.Reset(next)
}

func firstRunDelay(ctx context.Context, job scheduledJob, worker *service.Worker, offset time.Duration) time.Duration {
	lastRun, ok, err := worker.LastRun(ctx, job.name)

	if err != nil {
		slog.Error("reading worker schedule failed, deferring first run by a full interval", "job", job.name, "error", err)
		return job.interval + offset
	}

	if !ok {
		return firstRunDelayFor(job.interval, 0, offset, false)
	}

	return firstRunDelayFor(job.interval, time.Since(lastRun), offset, true)
}

func firstRunDelayFor(interval, sinceLastRun, offset time.Duration, hasLastRun bool) time.Duration {
	if !hasLastRun {
		return offset
	}

	remaining := interval - sinceLastRun
	if remaining < 0 {
		remaining = 0
	}

	return remaining + offset
}

func rescheduleDelay(interval, elapsed time.Duration) time.Duration {
	if next := interval - elapsed; next > 0 {
		return next
	}

	return 0
}

func executeJob(ctx context.Context, job scheduledJob, worker *service.Worker) {
	recordRun := func(count int64, runErr error) {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if writeErr := worker.RecordRun(writeCtx, job.name, count, runErr); writeErr != nil {
			slog.Error("recording worker run failed", "job", job.name, "error", writeErr)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error(job.name+" worker panicked", "panic", r, "stack", string(debug.Stack()))
			recordRun(0, fmt.Errorf("panicked: %v", r))
		}
	}()

	worker.Begin(job.name)
	defer worker.End(job.name)

	count, err := job.run(ctx)

	if ctx.Err() != nil {
		return
	}

	if err != nil {
		slog.Error(job.name+" tick failed", "error", err)
	}

	if count > 0 && job.doneMsg != "" {
		slog.Info(job.doneMsg, "count", count)
	}

	recordRun(int64(count), err)
}

func runImageTagRefresh(ctx context.Context, images *service.Images) (int, error) {
	slog.Info("refreshing image tags for all configured images")

	start := time.Now()

	refreshed, err := images.RefreshAll(ctx)

	if err != nil {
		return refreshed, err
	}

	if refreshed > 0 {
		slog.Info("image tag refresh completed", "refreshed", refreshed, "elapsed", time.Since(start))
	}

	return refreshed, nil
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
