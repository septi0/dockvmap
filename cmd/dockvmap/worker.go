package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/service"
)

const sessionCleanupInterval = time.Hour
const tagNotificationInterval = 5 * time.Minute
const blobOrphanScanInterval = 24 * time.Hour
const tagDiscoveryCleanupInterval = 24 * time.Hour
const tagDiscoveryRetention = 7 * 24 * time.Hour

const workerStartStagger = 10 * time.Second

type scheduledJob struct {
	name     string
	interval time.Duration
	run      func(context.Context) error
}

func startWorker(ctx context.Context, cfg *config.Config, schedule *service.WorkerSchedule, images *service.Images, discoveries *service.Discoveries, cache *blobcache.Cache, notifications *service.Notifications, sessions *service.Sessions) {
	jobs := []scheduledJob{
		{
			name:     "session-cleanup",
			interval: sessionCleanupInterval,
			run:      func(ctx context.Context) error { return runSessionCleanup(ctx, sessions) },
		},
		{
			name:     "tag-discovery-cleanup",
			interval: tagDiscoveryCleanupInterval,
			run:      func(ctx context.Context) error { return runTagDiscoveryCleanup(ctx, discoveries) },
		},
	}

	if interval, err := time.ParseDuration(cfg.TagsCheckInterval); err == nil && interval > 0 {
		jobs = append(jobs, scheduledJob{
			name:     service.WorkerJobTagRefresh,
			interval: interval,
			run:      func(ctx context.Context) error { return runImageTagRefresh(ctx, images) },
		})
	} else {
		slog.Warn("image tag refresh worker disabled", "interval", cfg.TagsCheckInterval)
	}

	if cache != nil {
		if interval, err := time.ParseDuration(cfg.BlobCache.CleanupInterval); err == nil && interval > 0 {
			jobs = append(jobs, scheduledJob{
				name:     "blob-cache-cleanup",
				interval: interval,
				run:      func(ctx context.Context) error { return runBlobCacheCleanup(ctx, cache) },
			})
		} else {
			slog.Warn("blob cache cleanup worker disabled", "interval", cfg.BlobCache.CleanupInterval)
		}

		jobs = append(jobs, scheduledJob{
			name:     "blob-cache-orphan-scan",
			interval: blobOrphanScanInterval,
			run:      func(ctx context.Context) error { return runBlobCacheOrphanScan(ctx, cache) },
		})
	}

	if cfg.SMTP.Enabled || len(cfg.Webhooks) > 0 {
		jobs = append(jobs, scheduledJob{
			name:     "tag-notification",
			interval: tagNotificationInterval,
			run:      func(ctx context.Context) error { return runTagNotification(ctx, notifications) },
		})
	}

	var wg sync.WaitGroup

	for i, job := range jobs {
		offset := time.Duration(i) * workerStartStagger

		slog.Info("starting background worker", "job", job.name, "interval", job.interval, "start_delay", offset)

		wg.Add(1)

		go func() {
			defer wg.Done()
			runScheduledJob(ctx, job, schedule, offset)
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
