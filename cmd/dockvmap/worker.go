package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/service"
)

const sessionCleanupInterval = time.Hour
const tagNotificationInterval = 5 * time.Minute

func startWorker(ctx context.Context, cfg *config.Config, images *service.Images, cache *blobcache.Cache, notifications *service.Notifications, sessions *service.Sessions) {
	var wg sync.WaitGroup

	slog.Info("starting session cleanup worker", "interval", sessionCleanupInterval)

	wg.Add(1)

	go func() {
		defer wg.Done()
		runSessionCleanupWorker(ctx, sessionCleanupInterval, sessions)
	}()

	if imageInterval, err := time.ParseDuration(cfg.TagsCheckInterval); err == nil && imageInterval > 0 {
		slog.Info("starting image tag refresh worker", "interval", imageInterval)

		wg.Add(1)

		go func() {
			defer wg.Done()
			runImageTagRefreshWorker(ctx, imageInterval, images)
		}()
	} else {
		slog.Warn("image tag refresh worker disabled", "interval", cfg.TagsCheckInterval)
	}

	if cache != nil {
		if cleanupInterval, err := time.ParseDuration(cfg.BlobCache.CleanupInterval); err == nil && cleanupInterval > 0 {
			slog.Info("starting blob cache cleanup worker", "interval", cleanupInterval)

			wg.Add(1)

			go func() {
				defer wg.Done()
				runBlobCacheCleanupWorker(ctx, cleanupInterval, cache)
			}()
		} else {
			slog.Warn("blob cache cleanup worker disabled", "interval", cfg.BlobCache.CleanupInterval)
		}
	}

	if cfg.SMTP.Enabled || len(cfg.Webhooks) > 0 {
		slog.Info("starting tag notification worker", "interval", tagNotificationInterval)

		wg.Add(1)

		go func() {
			defer wg.Done()
			runTagNotificationWorker(ctx, tagNotificationInterval, notifications)
		}()
	}

	wg.Wait()
}

func runTickerLoop(ctx context.Context, interval time.Duration, name string, fn func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				slog.Error(name+" tick failed", "error", err)
			}

		case <-ctx.Done():
			slog.Info(name + " worker stopped")
			return
		}
	}
}

func runImageTagRefreshWorker(ctx context.Context, interval time.Duration, images *service.Images) {
	runTickerLoop(ctx, interval, "image tag refresh", func(ctx context.Context) error {
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
	})
}

func runTagNotificationWorker(ctx context.Context, interval time.Duration, notifications *service.Notifications) {
	runTickerLoop(ctx, interval, "tag notification", func(ctx context.Context) error {
		sent, err := notifications.SendPendingTagNotifications(ctx)

		if err != nil {
			return err
		}

		if sent > 0 {
			slog.Info("sent tag notifications", "count", sent)
		}

		return nil
	})
}

func runSessionCleanupWorker(ctx context.Context, interval time.Duration, sessions *service.Sessions) {
	runTickerLoop(ctx, interval, "session cleanup", func(ctx context.Context) error {
		deleted, err := sessions.CleanupExpired(ctx)

		if err != nil {
			return err
		}

		if deleted > 0 {
			slog.Info("removed expired sessions", "count", deleted)
		}

		return nil
	})
}

func runBlobCacheCleanupWorker(ctx context.Context, interval time.Duration, cache *blobcache.Cache) {
	runTickerLoop(ctx, interval, "blob cache cleanup", func(ctx context.Context) error {
		deleted, err := cache.Cleanup(ctx)

		if err != nil {
			return err
		}

		if deleted > 0 {
			slog.Info("removed expired cached blobs", "count", deleted)
		}

		return nil
	})
}
