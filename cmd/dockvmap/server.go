package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/web"
)

const shutdownTimeout = 10 * time.Second

func listenAndServe(srv *http.Server, name string, errs chan<- error) {
	slog.Info("starting server", "name", name, "address", srv.Addr)

	var err error

	if srv.TLSConfig != nil {
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}

	if !errors.Is(err, http.ErrServerClosed) {
		errs <- fmt.Errorf("%s server: %w", name, err)
	}
}

func loadTLSConfig(cfg *config.Config) (*tls.Config, error) {
	if !cfg.TLS.Enabled {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)

	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	slog.Info("tls enabled")

	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}

func awaitShutdown(proxySrv, webSrv *http.Server, workerCancel context.CancelFunc, workerDone <-chan struct{}, serverErrs <-chan error) {
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)

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
}

func newProxyServer(cfg *config.Config, images *service.Images, ociClient *oci.Client, cache *blobcache.Cache, metrics *proxy.Metrics, proxyTokens *service.ProxyTokens, tlsConfig *tls.Config) *http.Server {
	return &http.Server{
		Addr:              cfg.ProxyServerListen,
		Handler:           proxy.New(cfg, images, ociClient, cache, metrics, proxyTokens),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    10 << 20,
		TLSConfig:         tlsConfig,
	}
}

func newWebServer(cfg *config.Config, images *service.Images, registries *service.Registries, events *service.Events, audit *service.Audit, users *service.Users, sessions *service.Sessions, health *service.Health, proxyTokens *service.ProxyTokens, metrics *proxy.Metrics, failures *service.FailureLog, loginRateLimitWindow time.Duration, version string, tlsConfig *tls.Config) (*http.Server, error) {
	handler, err := web.New(cfg, images, registries, events, audit, users, sessions, health, proxyTokens, metrics, failures, loginRateLimitWindow, version)

	if err != nil {
		return nil, err
	}

	return &http.Server{
		Addr:              cfg.WebServerListen,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig:         tlsConfig,
	}, nil
}
