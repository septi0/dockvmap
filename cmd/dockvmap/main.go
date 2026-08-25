package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to configuration file (optional; falls back to DOCKVMAP_* env vars and defaults if not found)")
	filtersPath := flag.String("filters", "filters.yaml", "path to tag-filters file (optional; falls back to the built-in default if not found)")
	resetPassword := flag.String("reset-password", "", "generate a new random password for the given username, print it, and exit")
	refreshTags := flag.Bool("refresh-tags", false, "refresh tags for all configured images from their upstream registries, then exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*configPath)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	tagFilter, err := tagfilter.Load(*filtersPath)

	if err != nil {
		return fmt.Errorf("failed to load tag filters: %w", err)
	}

	logFile, err := setupLogging(cfg.LogsPath)

	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	if logFile != nil {
		defer logFile.Close()
	}

	slog.Info("starting dockvmap", "version", version)

	if err := os.MkdirAll(cfg.DataPath, 0o755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	credentialEncryptionKey, err := resolveCredentialEncryptionKey(cfg)

	if err != nil {
		return fmt.Errorf("failed to resolve credential encryption key: %w", err)
	}

	db, err := store.New(filepath.Join(cfg.DataPath, "dockvmap.db"), credentialEncryptionKey)

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	defer db.Close()

	loginRateLimitWindow := cfg.LoginRateLimit.WindowDuration()

	loginRateLimiter, err := service.NewLoginRateLimiter(*cfg.LoginRateLimit.Enabled, cfg.LoginRateLimit.MaxAttempts, loginRateLimitWindow, cfg.LoginRateLimit.BypassIPs)

	if err != nil {
		return fmt.Errorf("failed to configure login rate limiter: %w", err)
	}

	audit := service.NewAudit(db)
	sessions := service.NewSessions(db, cfg.SessionLifetimeDuration(), audit, loginRateLimiter)
	users := service.NewUsers(db, audit, sessions)

	if *resetPassword != "" {
		return runResetPassword(context.Background(), users, *resetPassword)
	}

	registries := service.NewRegistries(db, audit)

	credentialsAdapter := registryCredentialsAdapter{registries: registries}
	optionsAdapter := registryOptionsAdapter{registries: registries}
	ociClient := oci.NewClient(nil, credentialsAdapter, optionsAdapter)

	failureLog := service.NewFailureLog()

	events := service.NewEvents(db)
	images := service.NewImages(db, ociClient, events, audit, failureLog, tagFilter)

	if *refreshTags {
		return runRefreshTags(context.Background(), images)
	}

	health := service.NewHealth(db)
	proxyTokens := service.NewProxyTokens(db, audit)
	metrics := proxy.NewMetrics()

	cache, err := initBlobCache(cfg, db)

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

	webSrv, err := newWebServer(cfg, images, registries, events, audit, users, sessions, health, proxyTokens, metrics, failureLog, loginRateLimitWindow, version, tlsConfig)

	if err != nil {
		return fmt.Errorf("failed to initialize web server: %w", err)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())

	defer workerCancel()

	serverErrs := make(chan error, 2)

	go listenAndServe(proxySrv, "proxy", serverErrs)
	go listenAndServe(webSrv, "web", serverErrs)

	workerDone := make(chan struct{})

	go func() {
		defer close(workerDone)
		startWorker(workerCtx, cfg, images, cache, notifications, sessions)
	}()

	awaitShutdown(proxySrv, webSrv, workerCancel, workerDone, serverErrs)

	return nil
}

func runResetPassword(ctx context.Context, users *service.Users, username string) error {
	password, err := users.ResetPassword(ctx, username)

	if err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	fmt.Println("Password reset. New password:")
	fmt.Println(password)

	return nil
}

func runRefreshTags(ctx context.Context, images *service.Images) error {
	refreshed, err := images.RefreshAll(ctx)

	fmt.Printf("Refreshed tags for %d image(s).\n", refreshed)

	if err != nil {
		return fmt.Errorf("some images failed to refresh: %w", err)
	}

	return nil
}

func setupLogging(logsPath string) (*os.File, error) {
	if logsPath == "" {
		return nil, nil
	}

	if err := os.MkdirAll(logsPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating logs directory: %w", err)
	}

	logFile, err := os.OpenFile(filepath.Join(logsPath, "dockvmap.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile), nil))
	slog.SetDefault(logger)

	return logFile, nil
}
