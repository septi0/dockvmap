package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const (
	cacheResetInterval = time.Hour
	firstRetryDelay    = 5 * time.Second
	maxRetryDelay      = time.Minute
	shutdownTimeout    = 5 * time.Second
)

var containerChanges = []string{"create", "start", "die", "destroy", "rename", "pause", "unpause", "update", "health_status"}

type Agent struct {
	cfg         Config
	version     string
	docker      *dockerClient
	cache       *dockerCache
	identity    tls.Certificate
	fingerprint string

	mu     sync.Mutex
	paired pairing

	snapshotMu  sync.Mutex
	dockerError string
}

func New(cfg Config, version string) (*Agent, error) {
	docker, err := newDockerClient(cfg.DockerHost)

	if err != nil {
		return nil, err
	}

	identity, err := loadIdentity(cfg.DataPath)

	if err != nil {
		return nil, err
	}

	paired, err := loadPairing(cfg.DataPath)

	if err != nil {
		return nil, err
	}

	return &Agent{
		cfg:         cfg,
		version:     version,
		docker:      docker,
		cache:       newDockerCache(),
		identity:    identity,
		fingerprint: agentapi.Fingerprint(identity.Leaf),
		paired:      paired,
	}, nil
}

func (a *Agent) Fingerprint() string {
	return a.fingerprint
}

func (a *Agent) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.cfg.Listen)

	if err != nil {
		return fmt.Errorf("listening on %s: %w", a.cfg.Listen, err)
	}

	return a.Serve(ctx, listener)
}

func (a *Agent) Serve(ctx context.Context, listener net.Listener) error {
	go a.watchEvents(ctx)
	go a.resetCache(ctx)

	if paired := a.pairedWith(); paired == "" {
		slog.Info("waiting for DockVMap to adopt this agent", "listen", listener.Addr().String())
	} else {
		slog.Info("paired with DockVMap", "dockvmap", paired)
	}

	server := &http.Server{
		Handler: a.handler(),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{a.identity},
			ClientAuth:   tls.RequireAnyClientCert,
			MinVersion:   tls.VersionTLS13,
		},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug),
	}

	errs := make(chan error, 1)

	go func() { errs <- server.ServeTLS(listener, "", "") }()

	select {
	case err := <-errs:
		return err

	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		slog.Info("agent stopped")

		return nil
	}
}

func (a *Agent) pairedWith() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.paired.DockVMap
}

func (a *Agent) resetCache(ctx context.Context) {
	ticker := time.NewTicker(cacheResetInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.cache.reset()
		}
	}
}

func (a *Agent) watchEvents(ctx context.Context) {
	delay := firstRetryDelay

	onConnect := func() {
		a.cache.reset()
		a.cache.setWatching(true)
	}

	onEvent := func(event dockerEvent) {
		action, _, _ := strings.Cut(event.Action, ":")

		switch {
		case event.Type == "container" && slices.Contains(containerChanges, action):
			a.cache.forgetContainer(event.Actor.ID)
		case event.Type == "image":
			a.cache.forgetImages()
		}
	}

	for {
		connected := time.Now()

		err := a.docker.watchEvents(ctx, onConnect, onEvent)

		a.cache.setWatching(false)

		if ctx.Err() != nil {
			return
		}

		if time.Since(connected) > maxRetryDelay {
			delay = firstRetryDelay
		}

		slog.Warn("watching Docker events failed; every poll reads Docker in full until it reconnects", "error", err, "retry", delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		delay = min(delay*2, maxRetryDelay)
	}
}
