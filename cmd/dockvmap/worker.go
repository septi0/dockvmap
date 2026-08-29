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
	run        func(context.Context) (int, error)
	doneMsg    string // logged with "count" when run reports count > 0
	onShutdown func(context.Context) error
	trigger    <-chan struct{} // nil unless the job can be run early on demand
}

func counted(n int64, err error) (int, error) { return int(n), err }

type workerDeps struct {
	cfg                 *config.Config
	schedule            *service.WorkerSchedule
	trigger             *service.WorkerTrigger
	activity            *service.WorkerActivity
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
			doneMsg:  "removed expired sessions",
			run:      func(ctx context.Context) (int, error) { return counted(deps.sessions.CleanupExpired(ctx)) },
		},
		{
			name:     "tag-discovery-cleanup",
			interval: tagDiscoveryCleanupInterval,
			doneMsg:  "removed old tag discoveries",
			run: func(ctx context.Context) (int, error) {
				return counted(deps.discoveries.CleanupOld(ctx, tagDiscoveryRetention))
			},
		},
		{
			name:     "background-failure-cleanup",
			interval: backgroundFailureCleanupInterval,
			doneMsg:  "removed old background failures",
			run: func(ctx context.Context) (int, error) {
				return counted(deps.failures.CleanupOld(ctx, backgroundFailureRetention))
			},
		},
	}

	if interval := cfg.TagsCheckIntervalDuration(); interval > 0 {
		jobs = append(jobs, scheduledJob{
			name:     service.WorkerJobTagRefresh,
			interval: interval,
			trigger:  deps.trigger.Channel(service.WorkerJobTagRefresh),
			run:      func(ctx context.Context) (int, error) { return runImageTagRefresh(ctx, deps.images) },
		})
	} else {
		slog.Warn("image tag refresh worker disabled", "interval", cfg.TagsCheckInterval)
	}

	if deps.cache != nil {
		if interval, err := time.ParseDuration(cfg.BlobCache.CleanupInterval); err == nil && interval > 0 {
			jobs = append(jobs, scheduledJob{
				name:     "blob-cache-cleanup",
				interval: interval,
				doneMsg:  "removed expired cached blobs",
				run:      func(ctx context.Context) (int, error) { return deps.cache.Cleanup(ctx) },
			})
		} else {
			slog.Warn("blob cache cleanup worker disabled", "interval", cfg.BlobCache.CleanupInterval)
		}

		jobs = append(jobs, scheduledJob{
			name:     "blob-cache-orphan-scan",
			interval: blobOrphanScanInterval,
			doneMsg:  "removed orphaned cached blob files",
			run:      func(ctx context.Context) (int, error) { return deps.cache.ScanOrphans(ctx) },
		})
	}

	if cfg.SMTP.Enabled || len(cfg.Webhooks) > 0 {
		jobs = append(jobs, scheduledJob{
			name:     "tag-notification",
			interval: tagNotificationInterval,
			doneMsg:  "sent tag notifications",
			run:      func(ctx context.Context) (int, error) { return deps.notifications.SendPendingTagNotifications(ctx) },
		})
	}

	if deps.proxyMetrics != nil && deps.proxyMetricsHistory != nil {
		flusher := &proxyMetricsFlusher{metrics: deps.proxyMetrics, history: deps.proxyMetricsHistory}

		jobs = append(jobs,
			scheduledJob{
				name:       "proxy-metrics-flush",
				interval:   proxyMetricsFlushInterval,
				run:        func(ctx context.Context) (int, error) { return 0, flusher.flush(ctx) },
				onShutdown: flusher.flush,
			},
			scheduledJob{
				name:     "proxy-metrics-cleanup",
				interval: proxyMetricsCleanupInterval,
				doneMsg:  "removed old proxy metrics",
				run: func(ctx context.Context) (int, error) {
					return counted(deps.proxyMetricsHistory.CleanupOld(ctx, proxyMetricsRetention))
				},
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
			runScheduledJob(ctx, job, deps.schedule, deps.activity, offset)
		}()
	}

	wg.Wait()
}

func runScheduledJob(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule, activity *service.WorkerActivity, offset time.Duration) {
	timer := time.NewTimer(firstRunDelay(ctx, job, schedule, offset))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			runAndReschedule(ctx, job, schedule, activity, timer)

		case <-job.trigger:
			runAndReschedule(ctx, job, schedule, activity, timer)

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
func runAndReschedule(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule, activity *service.WorkerActivity, timer *time.Timer) {
	start := time.Now()

	executeJob(ctx, job, schedule, activity)

	next := job.interval - time.Since(start)
	if next < 0 {
		next = 0
	}

	timer.Reset(next)
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

func executeJob(ctx context.Context, job scheduledJob, schedule *service.WorkerSchedule, activity *service.WorkerActivity) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error(job.name+" worker panicked", "panic", r, "stack", string(debug.Stack()))
		}
	}()

	activity.Begin(job.name)
	defer activity.End(job.name)

	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := schedule.MarkRun(markCtx, job.name); err != nil {
		slog.Error("recording worker run failed", "job", job.name, "error", err)
	}

	count, err := job.run(ctx)

	if err != nil {
		slog.Error(job.name+" tick failed", "error", err)
	}

	if count > 0 && job.doneMsg != "" {
		slog.Info(job.doneMsg, "count", count)
	}
}

func runImageTagRefresh(ctx context.Context, images *service.Images) (int, error) {
	slog.Info("refreshing image tags for all configured images")

	start := time.Now()

	refreshed, err := images.RefreshAll(ctx)

	if err != nil {
		return 0, err
	}

	if refreshed > 0 {
		slog.Info("image tag refresh completed", "refreshed", refreshed, "elapsed", time.Since(start))
	}

	return 0, nil
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
