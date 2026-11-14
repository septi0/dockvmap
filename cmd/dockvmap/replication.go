package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/httpmw"
	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/ocicopy"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/replication"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
)

// the settings that change what a follower's proxy serves; the deletes setting is probed from the registry
func replicationSetup(cfg *config.Config) replicationapi.Setup {
	return replicationapi.Setup{
		"virtual_tag":               cfg.VirtualTag,
		"proxy_auth.enabled":        strconv.FormatBool(cfg.ProxyAuth.Enabled),
		"tls.enabled":               strconv.FormatBool(cfg.TLS.Enabled),
		"internal_registry.enabled": strconv.FormatBool(cfg.InternalRegistry.Enabled),
	}
}

func internalRegistryClient(internalRegistry *service.InternalRegistry) *ocicopy.Registry {
	return &ocicopy.Registry{
		BaseURL: internalRegistry.URL(""),
		Client:  &http.Client{Transport: proxy.InternalRegistryTransport()},
	}
}

// no WriteTimeout: a long-poll waits 25s and a layer copy takes as long as it takes; requests end with ctx at shutdown
func newReplicationServer(ctx context.Context, cfg *config.Config, handler http.Handler, identity tls.Certificate) *http.Server {
	return &http.Server{
		Addr:              cfg.Replication.Listen,
		Handler:           httpmw.Recover(handler),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{identity}, MinVersion: tls.VersionTLS13},
	}
}

func startFollower(ctx context.Context, cfg *config.Config, db *store.Store, tagsChanged func(names ...string), internalRegistry *service.InternalRegistry) (*service.Follower, error) {
	if !cfg.Follower.Enabled {
		return nil, nil
	}

	client, err := replication.NewClient(cfg.Follower.Primary, cfg.Follower.Token, version)

	if err != nil {
		return nil, err
	}

	deps := service.FollowerDeps{
		Store:       db,
		Client:      client,
		Setup:       replicationSetup(cfg),
		Primary:     cfg.Follower.Primary,
		TagsChanged: tagsChanged,
	}

	if address := internalRegistry.Address(); address != "" {
		deps.Mirror = followerMirror{primary: client.Registry(), local: *internalRegistryClient(internalRegistry)}
		deps.Internal = &model.LocalInternalRegistry{Address: address, Options: internalRegistry.Options()}
		deps.RegistryPing = internalRegistry.Ping
	}

	follower := service.NewFollower(deps)

	if err := follower.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting the follower: %w", err)
	}

	return follower, nil
}

func startReplication(ctx context.Context, cfg *config.Config, db *store.Store, audit *service.Audit, internalRegistry *service.InternalRegistry, dataPath string) (*service.Replication, *http.Server, error) {
	if !cfg.Replication.Enabled {
		return nil, nil, nil
	}

	allowed, err := ipmatch.Parse(cfg.Replication.AllowedIPs)

	if err != nil {
		return nil, nil, fmt.Errorf("parsing replication.allowed_ips: %w", err)
	}

	identity, err := loadReplicationIdentity(dataPath)

	if err != nil {
		return nil, nil, err
	}

	deps := service.ReplicationDeps{
		Store:       db,
		Audit:       audit,
		Setup:       replicationSetup(cfg),
		Fingerprint: agentapi.Fingerprint(identity.Leaf),
		Listen:      cfg.Replication.Listen,
	}

	var registry *ocicopy.Registry

	if internalRegistry.Address() != "" {
		registry = internalRegistryClient(internalRegistry)
		deps.DeletesAllowed = registry.DeletesAllowed
		deps.RegistryPing = internalRegistry.Ping
	}

	primary := service.NewReplication(deps)

	if err := primary.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("starting replication: %w", err)
	}

	handler := replication.NewHandler(primary, registry, allowed)

	return primary, newReplicationServer(ctx, cfg, handler, identity), nil
}

func followerOf(cfg *config.Config) string {
	if !cfg.Follower.Enabled {
		return ""
	}

	return cfg.Follower.Primary
}
