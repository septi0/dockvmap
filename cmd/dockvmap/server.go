package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/web"
)

func listenAndServe(srv *http.Server, name string, errs chan<- error) {
	slog.Info("starting server", "name", name, "address", srv.Addr)

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		errs <- fmt.Errorf("%s server: %w", name, err)
	}
}

func newProxyServer(cfg *config.Config, images *service.Images, ociClient *oci.Client, cache *blobcache.Cache, metrics *proxy.Metrics, proxyTokens *service.ProxyTokens) *http.Server {
	return &http.Server{
		Addr:              cfg.ProxyServerListen,
		Handler:           proxy.New(cfg, images, ociClient, cache, metrics, proxyTokens),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    10 << 20,
	}
}

func newWebServer(cfg *config.Config, images *service.Images, registries *service.Registries, events *service.Events, audit *service.Audit, users *service.Users, sessions *service.Sessions, health *service.Health, proxyTokens *service.ProxyTokens, metrics *proxy.Metrics, failures *service.FailureLog, loginRateLimitWindow time.Duration) (*http.Server, error) {
	handler, err := web.New(cfg, images, registries, events, audit, users, sessions, health, proxyTokens, metrics, failures, loginRateLimitWindow)

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
	}, nil
}
