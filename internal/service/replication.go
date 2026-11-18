package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const (
	replicationWaitTimeout       = 25 * time.Second
	replicationWatchInterval     = time.Second
	replicationFollowerSeenTTL   = 2 * time.Minute
	replicationDeletesProbeEvery = 10 * time.Minute

	setupRegistryDeletes = "REGISTRY_STORAGE_DELETE_ENABLED"
)

type replicationStore interface {
	Ping(ctx context.Context) error
	ReplicationState(ctx context.Context) (model.ReplicationState, error)
	EnsureClusterID(ctx context.Context, newID string) (string, error)
	ReplicationSnapshot(ctx context.Context) (*model.ReplicationSnapshot, error)
	CleanupReplicationChanges(ctx context.Context, olderThan time.Time) (int64, error)
	RepositoryChanges(ctx context.Context, repository string, after int64) ([]model.RepositoryChange, bool, error)
	RecordProxyUsage(ctx context.Context, images []model.ImageUsage, tokens []model.ProxyTokenUsage) error
	CreateReplicationToken(ctx context.Context, label, tokenHash string) (int64, error)
	ListReplicationTokens(ctx context.Context) ([]model.ReplicationToken, error)
	ReplicationTokenByHash(ctx context.Context, tokenHash string) (*model.ReplicationToken, error)
	GetReplicationTokenByID(ctx context.Context, id int64) (*model.ReplicationToken, error)
	TouchReplicationToken(ctx context.Context, id int64, usedAt time.Time) error
	DeleteReplicationToken(ctx context.Context, id int64) (bool, error)
	SaveReplicationFollower(ctx context.Context, follower model.ReplicationFollower) error
	ListReplicationFollowers(ctx context.Context) ([]model.ReplicationFollower, error)
	GetReplicationFollower(ctx context.Context, id int64) (*model.ReplicationFollower, error)
	RenameReplicationFollower(ctx context.Context, id int64, name string) error
	DeleteReplicationFollower(ctx context.Context, id int64) (bool, error)
}

type ReplicationDeps struct {
	Store       replicationStore
	Audit       auditRecorder
	Setup       replicationapi.Setup
	Fingerprint string
	Listen      string

	// both check the internal registry; nil without one
	DeletesAllowed func(ctx context.Context) (bool, error)
	RegistryPing   func(ctx context.Context) error
}

type StateRequest struct {
	Version    int64
	Tag        string
	Cluster    string
	HasVersion bool
	Full       bool
}

type ReplicationStatus struct {
	ClusterID   string
	Version     int64
	Fingerprint string
	Listen      string
	Followers   []SeenFollower
}

type Replication struct {
	store         replicationStore
	audit         auditRecorder
	setup         replicationapi.Setup
	fingerprint   string
	listen        string
	deletes       *deletesProbe
	registryPing  func(ctx context.Context) error
	waitTimeout   time.Duration
	watchInterval time.Duration

	mu        sync.Mutex
	changed   chan struct{}
	waiting   int
	version   int64
	startedAt time.Time
	followers map[string]*liveFollower
}

func NewReplication(deps ReplicationDeps) *Replication {
	return &Replication{
		store:         deps.Store,
		audit:         deps.Audit,
		setup:         deps.Setup,
		fingerprint:   deps.Fingerprint,
		listen:        deps.Listen,
		deletes:       newDeletesProbe(deps.DeletesAllowed),
		registryPing:  deps.RegistryPing,
		waitTimeout:   replicationWaitTimeout,
		watchInterval: replicationWatchInterval,
		changed:       make(chan struct{}),
		startedAt:     time.Now(),
		followers:     make(map[string]*liveFollower),
	}
}

func (r *Replication) Start(ctx context.Context) error {
	clusterID, err := r.store.EnsureClusterID(ctx, newRandomID())

	if err != nil {
		return err
	}

	state, err := r.store.ReplicationState(ctx)

	if err != nil {
		return err
	}

	r.version = state.Version

	slog.Info("serving replication", "listen", r.listen, "cluster", clusterID, "fingerprint", r.fingerprint)

	go r.watch(ctx)

	return nil
}

func newRandomID() string {
	raw := make([]byte, 16)
	rand.Read(raw)

	return hex.EncodeToString(raw)
}

// one row read per second, only while a follower waits; it also sees changes made by another process
func (r *Replication) watch(ctx context.Context) {
	ticker := time.NewTicker(r.watchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		r.mu.Lock()
		waiting := r.waiting
		r.mu.Unlock()

		if waiting == 0 {
			continue
		}

		state, err := r.store.ReplicationState(ctx)

		if err != nil {
			slog.Warn("reading the replication version failed", "error", err)
			continue
		}

		r.mu.Lock()

		if state.Version != r.version {
			r.version = state.Version
			close(r.changed)
			r.changed = make(chan struct{})
		}

		r.mu.Unlock()
	}
}

// State answers with the snapshot, or nil and nil when nothing changed within the wait
func (r *Replication) State(ctx context.Context, req StateRequest) (*replicationapi.StateResponse, *replicationapi.Conflict, error) {
	state, err := r.store.ReplicationState(ctx)

	if err != nil {
		return nil, nil, err
	}

	if conflict := stateConflict(state, req); conflict != nil {
		return nil, conflict, nil
	}

	if !req.Full && req.Version == state.Version && !r.wait(ctx, state.Version) {
		return nil, nil, nil
	}

	snapshot, err := r.store.ReplicationSnapshot(ctx)

	if err != nil {
		return nil, nil, err
	}

	return &replicationapi.StateResponse{
		APIVersion: replicationapi.APIVersion,
		Setup:      r.currentSetup(ctx),
		Snapshot:   *snapshot,
	}, nil, nil
}

func stateConflict(state model.ReplicationState, req StateRequest) *replicationapi.Conflict {
	switch {
	case req.Cluster != "" && req.Cluster != state.ClusterID:
		return &replicationapi.Conflict{Reason: replicationapi.ConflictOtherCluster, PrimaryVersion: state.Version}
	case !req.HasVersion:
		return nil
	case req.Version > state.Version, req.Version == state.Version && req.Tag != state.Tag:
		return &replicationapi.Conflict{Reason: replicationapi.ConflictDiverged, PrimaryVersion: state.Version}
	}

	return nil
}

func (r *Replication) wait(ctx context.Context, version int64) bool {
	r.mu.Lock()
	r.version = max(r.version, version)
	changed := r.changed
	newer := r.version != version
	r.waiting++
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.waiting--
		r.mu.Unlock()
	}()

	if newer {
		return true
	}

	timer := time.NewTimer(r.waitTimeout)
	defer timer.Stop()

	select {
	case <-changed:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
	}

	state, err := r.store.ReplicationState(ctx)

	return err == nil && state.Version != version
}

func (r *Replication) currentSetup(ctx context.Context) replicationapi.Setup {
	return r.deletes.addTo(ctx, r.setup)
}

type deletesProbe struct {
	probe func(ctx context.Context) (bool, error)

	mu      sync.Mutex
	known   bool
	allowed bool
	checked time.Time
}

func newDeletesProbe(probe func(ctx context.Context) (bool, error)) *deletesProbe {
	return &deletesProbe{probe: probe}
}

// the last known answer is kept while the registry doesn't answer; unknown leaves the setting out of the comparison
func (p *deletesProbe) addTo(ctx context.Context, setup replicationapi.Setup) replicationapi.Setup {
	setup = maps.Clone(setup)

	if p.probe == nil {
		return setup
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if time.Since(p.checked) > replicationDeletesProbeEvery {
		if allowed, err := p.probe(ctx); err == nil {
			p.known, p.allowed, p.checked = true, allowed, time.Now()
		} else {
			slog.Debug("checking whether the internal registry allows deletes failed", "error", err)
		}
	}

	if p.known {
		setup[setupRegistryDeletes] = fmt.Sprint(p.allowed)
	}

	return setup
}

func (r *Replication) Status(ctx context.Context) (*ReplicationStatus, error) {
	state, err := r.store.ReplicationState(ctx)

	if err != nil {
		return nil, err
	}

	followers, err := r.Followers(ctx)

	if err != nil {
		return nil, err
	}

	return &ReplicationStatus{
		ClusterID:   state.ClusterID,
		Version:     state.Version,
		Fingerprint: r.fingerprint,
		Listen:      r.listen,
		Followers:   followers,
	}, nil
}

func (r *Replication) Ready(ctx context.Context) (bool, map[string]string) {
	checks := map[string]string{"database": "ok"}

	if err := r.store.Ping(ctx); err != nil {
		checks["database"] = err.Error()
	}

	if r.registryPing != nil {
		checks["registry"] = "ok"

		if err := r.registryPing(ctx); err != nil {
			checks["registry"] = err.Error()
		}
	}

	return allOK(checks), checks
}

func allOK(checks map[string]string) bool {
	for _, result := range checks {
		if result != "ok" {
			return false
		}
	}

	return true
}

func (r *Replication) CleanupChanges(ctx context.Context, retention time.Duration) (int64, error) {
	return r.store.CleanupReplicationChanges(ctx, time.Now().Add(-retention))
}

func (r *Replication) Changes(ctx context.Context, repository string, after int64) (*replicationapi.ChangesResponse, error) {
	changes, complete, err := r.store.RepositoryChanges(ctx, repository, after)

	if err != nil {
		return nil, err
	}

	return &replicationapi.ChangesResponse{Changes: changes, Complete: complete}, nil
}

// RecordPulls merges a follower's pulls into this primary's pull tracking; a tag that can't be one is dropped
func (r *Replication) RecordPulls(ctx context.Context, report replicationapi.PullReport) error {
	images := slices.DeleteFunc(report.Images, func(u model.ImageUsage) bool { return !oci.ValidTag(u.PulledTag) })

	return r.store.RecordProxyUsage(ctx, images, report.Tokens)
}
