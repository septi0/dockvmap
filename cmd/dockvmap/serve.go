package main

import (
	"context"
	"fmt"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/tagfilter"
	"github.com/septi0/dockvmap/internal/web"
)

func serve(ctx context.Context, cfg *config.Config, db *store.Store, dataPath string, tagFilter *tagfilter.Filter) error {
	loginRateLimitWindow := cfg.LoginRateLimit.WindowDuration()

	loginRateLimiter, err := service.NewLoginRateLimiter(*cfg.LoginRateLimit.Enabled, cfg.LoginRateLimit.MaxAttempts, loginRateLimitWindow, cfg.LoginRateLimit.BypassIPs)

	if err != nil {
		return fmt.Errorf("failed to configure login rate limiter: %w", err)
	}

	audit := service.NewAudit(db)
	sessions := service.NewSessions(db, cfg.SessionLifetimeDuration(), audit, loginRateLimiter)
	users := service.NewUsers(db, audit, sessions)

	registries := service.NewRegistries(db, audit)
	ociClient := oci.NewClient(nil, registryCredentialsAdapter{registries: registries}, registryOptionsAdapter{registries: registries})

	failureLog := service.NewFailureLog(db)

	workerCtx, workerCancel := context.WithCancel(ctx)

	defer workerCancel()

	events := service.NewEvents(db)

	images := service.NewImages(service.ImagesDeps{
		Store:     db,
		TagLister: ociClient,
		Events:    events,
		Audit:     audit,
		Failures:  failureLog,
		TagFilter: tagFilter,
		BgCtx:     workerCtx,
	})

	discoveries := service.NewDiscoveries(service.DiscoveriesDeps{
		Store:      db,
		Registries: db,
		Checker:    ociClient,
		TagLister:  ociClient,
		TagFilter:  tagFilter,
		Failures:   failureLog,
		BgCtx:      workerCtx,
		TTL:        cfg.TagDiscoveryTTLDuration(),
	})

	if err := discoveries.RecoverFromRestart(ctx); err != nil {
		return fmt.Errorf("failed to recover tag discoveries: %w", err)
	}

	if err := images.RecoverFromRestart(ctx); err != nil {
		return fmt.Errorf("failed to recover image refresh state: %w", err)
	}

	health := service.NewHealth(db)
	proxyTokens := service.NewProxyTokens(db, audit)
	workerSchedule := service.NewWorkerSchedule(db)

	var triggerableJobs []string

	if cfg.TagsCheckIntervalDuration() > 0 {
		triggerableJobs = append(triggerableJobs, service.WorkerJobTagRefresh)
	}

	workerTrigger := service.NewWorkerTrigger(triggerableJobs...)
	workerActivity := service.NewWorkerActivity()
	proxyMetricsHistory := service.NewProxyMetricsHistory(db)
	metrics := proxy.NewMetrics()

	cache, err := initBlobCache(cfg, dataPath, db)

	if err != nil {
		return err
	}

	mailer := initMailer(cfg)

	notifications, err := initNotifications(cfg, db, mailer, failureLog)

	if err != nil {
		return err
	}

	tlsConfig, err := loadTLSConfig(cfg)

	if err != nil {
		return err
	}

	proxySrv := newProxyServer(cfg, images, ociClient, cache, metrics, proxyTokens, tlsConfig)

	webDeps := web.Dependencies{
		Config:               cfg,
		Images:               images,
		Discoveries:          discoveries,
		Registries:           registries,
		Events:               events,
		Audit:                audit,
		Users:                users,
		Sessions:             sessions,
		Health:               health,
		ProxyTokens:          proxyTokens,
		ProxyMetricsHistory:  proxyMetricsHistory,
		Failures:             failureLog,
		WorkerSchedule:       workerSchedule,
		WorkerTrigger:        workerTrigger,
		WorkerActivity:       workerActivity,
		LoginRateLimitWindow: loginRateLimitWindow,
		Version:              version,
	}

	if cache != nil {
		webDeps.CacheUsage = cache
	}

	webSrv, err := newWebServer(webDeps, tlsConfig)

	if err != nil {
		return fmt.Errorf("failed to initialize web server: %w", err)
	}

	serverErrs := make(chan error, 2)

	go listenAndServe(proxySrv, "proxy", serverErrs)
	go listenAndServe(webSrv, "web", serverErrs)

	workerDone := make(chan struct{})

	go func() {
		defer close(workerDone)
		startWorker(workerCtx, workerDeps{
			cfg:                 cfg,
			schedule:            workerSchedule,
			trigger:             workerTrigger,
			activity:            workerActivity,
			failures:            failureLog,
			images:              images,
			discoveries:         discoveries,
			cache:               cache,
			notifications:       notifications,
			sessions:            sessions,
			proxyMetrics:        metrics,
			proxyMetricsHistory: proxyMetricsHistory,
		})
	}()

	awaitShutdown(proxySrv, webSrv, workerCancel, workerDone, serverErrs)

	return nil
}
