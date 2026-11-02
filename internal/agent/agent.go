package agent

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const (
	heartbeatInterval  = 60 * time.Second
	cacheResetInterval = time.Hour
	eventDebounce      = 2 * time.Second
	firstRetryDelay    = 5 * time.Second
)

var containerChanges = []string{"create", "start", "die", "destroy", "rename", "pause", "unpause", "update", "health_status"}

type Agent struct {
	cfg             Config
	version         string
	docker          *dockerClient
	controller      *controllerClient
	cache           *dockerCache
	proxyHost       string
	lastFingerprint string
	lastError       string
}

func New(cfg Config, version string) (*Agent, error) {
	docker, err := newDockerClient(cfg.DockerHost)

	if err != nil {
		return nil, err
	}

	return &Agent{
		cfg:        cfg,
		version:    version,
		docker:     docker,
		controller: newControllerClient(cfg, version),
		cache:      newDockerCache(),
	}, nil
}

func (a *Agent) Run(ctx context.Context) {
	events := make(chan struct{}, 1)

	go a.watchEvents(ctx, events)

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	cacheReset := time.NewTicker(cacheResetInterval)
	defer cacheReset.Stop()

	var debounce, retry <-chan time.Time
	retryDelay := firstRetryDelay

	syncNow := func() {
		if err := a.sync(ctx); err != nil {
			a.logFailure(err)
			retry = time.After(retryDelay)
			retryDelay = min(retryDelay*2, heartbeatInterval)

			return
		}

		a.logRecovery()
		retry, retryDelay = nil, firstRetryDelay
	}

	syncNow()

	for {
		select {
		case <-ctx.Done():
			slog.Info("agent stopped")
			return

		case <-events:
			if debounce == nil {
				debounce = time.After(eventDebounce)
			}

		case <-debounce:
			debounce = nil
			syncNow()

		case <-retry:
			retry = nil
			syncNow()

		case <-heartbeat.C:
			syncNow()

		case <-cacheReset.C:
			a.cache.reset()
		}
	}
}

func (a *Agent) sync(ctx context.Context) error {
	if a.proxyHost == "" {
		config, err := a.controller.config(ctx)

		if err != nil {
			return err
		}

		a.proxyHost = config.ProxyHost
		slog.Info("connected to dockvmap", "url", a.cfg.URL.Redacted(), "proxy", a.proxyHost)
	}

	report := a.snapshot(ctx)

	if report.Fingerprint == a.lastFingerprint {
		err := a.controller.heartbeat(ctx, agentapi.Heartbeat{
			ProtocolVersion: report.ProtocolVersion,
			AgentVersion:    report.AgentVersion,
			Fingerprint:     report.Fingerprint,
		})

		if err == nil {
			slog.Debug("heartbeat sent")
			return nil
		}

		if !isStatus(err, agentapi.FullReportNeededStatus) {
			return a.controllerFailed(err)
		}

		slog.Info("dockvmap asked for a full report")
	}

	if err := a.controller.report(ctx, report); err != nil {
		a.lastFingerprint = ""

		return a.controllerFailed(err)
	}

	a.lastFingerprint = report.Fingerprint
	slog.Debug("report sent", "containers", len(report.Containers), "dockerError", report.DockerError)

	return nil
}

func (a *Agent) controllerFailed(err error) error {
	var controllerErr *controllerError

	if errors.As(err, &controllerErr) {
		a.proxyHost = ""
	}

	return err
}

func (a *Agent) watchEvents(ctx context.Context, events chan<- struct{}) {
	delay := firstRetryDelay

	onEvent := func(event dockerEvent) {
		action, _, _ := strings.Cut(event.Action, ":")

		switch {
		case event.Type == "container" && slices.Contains(containerChanges, action):
			a.cache.forgetContainer(event.Actor.ID)
		case event.Type == "image":
			a.cache.forgetImages()
		default:
			return
		}

		select {
		case events <- struct{}{}:
		default:
		}
	}

	for {
		connected := time.Now()

		err := a.docker.watchEvents(ctx, a.cache.reset, onEvent)

		if ctx.Err() != nil {
			return
		}

		if time.Since(connected) > heartbeatInterval {
			delay = firstRetryDelay
		}

		slog.Warn("watching Docker events failed; changes are picked up by the next heartbeat until it reconnects", "error", err, "retry", delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		delay = min(delay*2, heartbeatInterval)
	}
}

func (a *Agent) logFailure(err error) {
	if err.Error() == a.lastError {
		slog.Debug("still failing", "error", err)
		return
	}

	a.lastError = err.Error()
	slog.Error("syncing with dockvmap failed", "error", err)
}

func (a *Agent) logRecovery() {
	if a.lastError != "" {
		slog.Info("syncing with dockvmap works again")
		a.lastError = ""
	}
}

func isStatus(err error, status int) bool {
	var controllerErr *controllerError

	return errors.As(err, &controllerErr) && controllerErr.status == status
}
