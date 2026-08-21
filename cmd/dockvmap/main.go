package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/smtp"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/webhook"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	resetPassword := flag.String("reset-password", "", "generate a new random password for the given username, print it, and exit")
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

	db, err := store.New(filepath.Join(cfg.DataPath, "dockvmap.db"), cfg.CredentialEncryptionKey)

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	defer db.Close()

	sessionLifetime, err := time.ParseDuration(cfg.SessionLifetime)

	if err != nil || sessionLifetime <= 0 {
		return fmt.Errorf("invalid session_lifetime %q: %w", cfg.SessionLifetime, err)
	}

	loginRateLimitWindow, err := time.ParseDuration(cfg.LoginRateLimit.Window)

	if err != nil || loginRateLimitWindow <= 0 {
		return fmt.Errorf("invalid login_rate_limit.window %q: %w", cfg.LoginRateLimit.Window, err)
	}

	if cfg.LoginRateLimit.MaxAttempts <= 0 {
		return fmt.Errorf("invalid login_rate_limit.max_attempts %d: must be positive", cfg.LoginRateLimit.MaxAttempts)
	}

	loginRateLimiter, err := service.NewLoginRateLimiter(*cfg.LoginRateLimit.Enabled, cfg.LoginRateLimit.MaxAttempts, loginRateLimitWindow, cfg.LoginRateLimit.BypassIPs)

	if err != nil {
		return fmt.Errorf("failed to configure login rate limiter: %w", err)
	}

	audit := service.NewAudit(db)
	sessions := service.NewSessions(db, sessionLifetime, audit, loginRateLimiter)
	users := service.NewUsers(db, audit, sessions)

	if *resetPassword != "" {
		password, err := users.ResetPassword(context.Background(), *resetPassword)

		if err != nil {
			return fmt.Errorf("failed to reset password: %w", err)
		}

		fmt.Println("Password reset. New password:")
		fmt.Println(password)

		return nil
	}

	health := service.NewHealth(db)
	registries := service.NewRegistries(db, audit)
	proxyTokens := service.NewProxyTokens(db, audit)

	credentialsAdapter := registryCredentialsAdapter{registries: registries}
	optionsAdapter := registryOptionsAdapter{registries: registries}
	ociClient := oci.NewClient(nil, credentialsAdapter, optionsAdapter)

	failureLog := service.NewFailureLog()

	events := service.NewEvents(db)
	images := service.NewImages(db, ociClient, events, audit, failureLog)
	metrics := proxy.NewMetrics()

	var cache *blobcache.Cache

	if cfg.BlobCache.Enabled {
		cache, err = blobcache.New(cfg.BlobCache.Path, cfg.BlobCache.Lifetime, db)

		if err != nil {
			return fmt.Errorf("failed to initialize blob cache: %w", err)
		}

		slog.Info("blob cache enabled", "path", cfg.BlobCache.Path, "lifetime", cache.Lifetime())
	}

	var mailer *smtp.Client

	if cfg.SMTP.Enabled {
		mailer = smtp.NewClient(smtp.Config{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
			TLS:      cfg.SMTP.TLS,
		})

		slog.Info("smtp enabled", "host", cfg.SMTP.Host, "port", cfg.SMTP.Port)
	}

	if len(cfg.Webhooks) > 0 {
		slog.Info("webhook notifications enabled", "count", len(cfg.Webhooks))
	}

	notifications, err := service.NewNotifications(db, db, mailer, cfg.SMTP.Enabled, webhook.NewClient(), cfg.Webhooks, failureLog)

	if err != nil {
		return fmt.Errorf("failed to configure webhook notifications: %w", err)
	}

	proxySrv := newProxyServer(cfg, images, ociClient, cache, metrics, proxyTokens)

	webSrv, err := newWebServer(cfg, images, registries, events, audit, users, sessions, health, proxyTokens, metrics, failureLog, loginRateLimitWindow)

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

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(sig)

	select {
	case <-sig:
		slog.Info("shutting down")
	case err := <-serverErrs:
		slog.Error("server failed, shutting down", "error", err)
	}

	workerCancel()
	<-workerDone

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer shutdownCancel()

	var shutdownWg sync.WaitGroup
	shutdownWg.Add(2)

	go func() {
		defer shutdownWg.Done()

		if err := proxySrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("proxy server shutdown error", "error", err)
		}
	}()

	go func() {
		defer shutdownWg.Done()

		if err := webSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("web server shutdown error", "error", err)
		}
	}()

	shutdownWg.Wait()

	slog.Info("stopped")

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
