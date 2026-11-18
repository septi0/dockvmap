package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const (
	FollowerSyncRunning   = "running"
	FollowerDiverged      = "diverged"
	FollowerOtherCluster  = "other-cluster"
	FollowerExistingData  = "existing-data"
	FollowerSetupMismatch = "setup-mismatch"
	FollowerIdentity      = "identity-changed"
	FollowerAPIVersion    = "api-version"

	followerStoppedCheck  = time.Minute
	followerResyncPoll    = 3 * time.Second
	followerMaxBackoff    = time.Minute
	followerCopyRetry     = 15 * time.Second
	followerMaxCopyRetry  = 5 * time.Minute
	followerHealthTimeout = 5 * time.Second
)

type followerStore interface {
	Ping(ctx context.Context) error
	FollowerState(ctx context.Context) (model.FollowerState, error)
	FollowerHasLocalData(ctx context.Context) (bool, error)
	ReplicatedImages(ctx context.Context) ([]model.ReplicatedImage, error)
	ApplyReplicationSnapshot(ctx context.Context, snapshot *model.ReplicationSnapshot, internal *model.LocalInternalRegistry) (int64, error)
	FollowerRepositories(ctx context.Context) (map[string]int64, error)
	SetFollowerRepository(ctx context.Context, repository string, counter int64) error
	MarkInitialCopyDone(ctx context.Context) error
	TakeResyncRequest(ctx context.Context) (bool, error)
	ClearClusterID(ctx context.Context) error
	ClearPruneRequest(ctx context.Context) error
	FollowerID(ctx context.Context, newID string) (string, error)
	ProxyUsage(ctx context.Context) ([]model.ImageUsage, []model.ProxyTokenUsage, error)
}

type primaryClient interface {
	State(ctx context.Context, req StateRequest, status string, health replicationapi.FollowerHealth) (*replicationapi.StateResponse, *replicationapi.Conflict, error)
	Changes(ctx context.Context, repository string, after int64) (*replicationapi.ChangesResponse, error)
	ReportPulls(ctx context.Context, report replicationapi.PullReport) error
}

// registryMirror reads the primary's internal registry and writes this follower's own
type registryMirror interface {
	Repositories(ctx context.Context) ([]string, error)
	Tags(ctx context.Context, repository string) ([]string, error)
	Copy(ctx context.Context, repository, tag string, deep bool) (bool, error)
	HasTag(ctx context.Context, repository, tag string) (bool, error)
	Delete(ctx context.Context, repository, reference string) error
	LocalTags(ctx context.Context) (map[string][]string, error)
	DeleteTags(ctx context.Context, repository string, tags []string) error
	DeletesAllowed(ctx context.Context) (bool, error)
}

type FollowerDeps struct {
	Store   followerStore
	Client  primaryClient
	Setup   replicationapi.Setup
	Primary string

	// all nil when this follower has no internal registry
	Mirror       registryMirror
	Internal     *model.LocalInternalRegistry
	RegistryPing func(ctx context.Context) error

	TagsChanged func(names ...string)
}

type FollowerStatus struct {
	Primary             string
	ClusterID           string
	Sync                string
	AppliedVersion      int64
	PrimaryVersion      int64
	LastContactAt       *time.Time
	LastAppliedAt       *time.Time
	PendingRepositories int
	FailedCopies        int
	ExtraTags           *int
	InitialCopyDone     bool
	LastError           string
	CopyError           string
	Mismatches          []replicationapi.Mismatch
}

type Follower struct {
	store        followerStore
	client       primaryClient
	setup        replicationapi.Setup
	deletes      *deletesProbe
	primary      string
	mirror       registryMirror
	internal     *model.LocalInternalRegistry
	registryPing func(ctx context.Context) error
	tagsChanged  func(names ...string)
	id           string

	applyMu sync.Mutex
	copyMu  sync.Mutex
	copies  chan struct{}

	mu             sync.Mutex
	sync           string
	lastError      string
	mismatches     []replicationapi.Mismatch
	lastContact    time.Time
	primaryVersion int64
	latest         *replicationapi.StateResponse
	pending        int
	failed         int
	extraTags      *int
	copyErrors     map[string]string
}

func NewFollower(deps FollowerDeps) *Follower {
	var probe func(ctx context.Context) (bool, error)

	if deps.Mirror != nil {
		probe = deps.Mirror.DeletesAllowed
	}

	return &Follower{
		store:        deps.Store,
		client:       deps.Client,
		setup:        deps.Setup,
		deletes:      newDeletesProbe(probe),
		primary:      deps.Primary,
		mirror:       deps.Mirror,
		internal:     deps.Internal,
		registryPing: deps.RegistryPing,
		tagsChanged:  deps.TagsChanged,
		copies:       make(chan struct{}, 1),
		sync:         FollowerSyncRunning,
		copyErrors:   make(map[string]string),
	}
}

// Start gives up any primary identity this node had, then syncs in the background until ctx ends
func (f *Follower) Start(ctx context.Context) error {
	if err := f.store.ClearClusterID(ctx); err != nil {
		return err
	}

	id, err := f.store.FollowerID(ctx, newRandomID())

	if err != nil {
		return err
	}

	f.id = id

	slog.Info("following a primary", "primary", f.primary)

	go f.run(ctx)
	go f.runPullReports(ctx)

	if f.mirror != nil {
		go f.runCopies(ctx)
	}

	return nil
}

func (f *Follower) run(ctx context.Context) {
	full, resync := true, false
	failures := 0
	var lastStoppedCheck time.Time

	for ctx.Err() == nil {
		if requested, err := f.store.TakeResyncRequest(ctx); err != nil {
			slog.Error("reading the resync request failed", "error", err)
		} else if requested {
			slog.Info("resyncing from the primary as requested", "primary", f.primary)
			full, resync = true, true
			f.resume()
		}

		if f.isStopped() {
			if time.Since(lastStoppedCheck) >= followerStoppedCheck {
				lastStoppedCheck = time.Now()
				f.reportWhileStopped(ctx)
			}

			sleep(ctx, followerResyncPoll)
			continue
		}

		if err := f.syncOnce(ctx, full, resync); err != nil {
			if ctx.Err() != nil {
				return
			}

			failures++
			f.setError(err)
			sleep(ctx, min(time.Duration(1<<min(failures-1, 6))*time.Second, followerMaxBackoff))

			continue
		}

		// copies that failed while the primary was unreachable resume as soon as it answers again
		if failures > 0 {
			f.queueCopies()
		}

		full, resync, failures = false, false, 0
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (f *Follower) request(ctx context.Context, full, resync bool) (StateRequest, model.FollowerState, error) {
	state, err := f.store.FollowerState(ctx)

	if err != nil {
		return StateRequest{}, state, err
	}

	req := StateRequest{Full: full}

	if !resync && state.ClusterID != "" {
		req.Version, req.Tag, req.Cluster, req.HasVersion = state.AppliedVersion, state.AppliedTag, state.ClusterID, true
	}

	return req, state, nil
}

func (f *Follower) syncOnce(ctx context.Context, full, resync bool) error {
	req, state, err := f.request(ctx, full, resync)

	if err != nil {
		return err
	}

	response, conflict, err := f.client.State(ctx, req, f.currentSync(), f.health(ctx))

	if err != nil {
		return err
	}

	f.contacted()

	if conflict != nil {
		f.stopOnConflict(conflict, state)
		return nil
	}

	if response == nil {
		f.clearError()
		return nil
	}

	f.setPrimaryVersion(response.Snapshot.State.Version)

	if mismatches := f.deletes.addTo(ctx, f.setup).Mismatches(response.Setup); len(mismatches) > 0 {
		f.stop(FollowerSetupMismatch, setupMismatchMessage(mismatches), mismatches)
		return nil
	}

	if !resync && state.ClusterID == "" {
		hasData, err := f.store.FollowerHasLocalData(ctx)

		if err != nil {
			return err
		}

		if hasData {
			f.stop(FollowerExistingData, "Sync stopped: this DockVMap already has its own virtual images, groups, registries or proxy tokens, and following the primary replaces them. To replace them, run dockvmap -resync on this follower's host.", nil)
			return nil
		}
	}

	f.mu.Lock()
	f.latest = response
	f.mu.Unlock()

	if err := f.applyState(ctx); err != nil {
		return err
	}

	if resync {
		slog.Info("resynced from the primary", "primary", f.primary, "version", response.Snapshot.State.Version)
	}

	f.clearError()
	f.queueCopies()

	return nil
}

func setupMismatchMessage(mismatches []replicationapi.Mismatch) string {
	parts := make([]string, 0, len(mismatches))

	for _, m := range mismatches {
		parts = append(parts, fmt.Sprintf("%s is %q on the primary and %q here", m.Setting, m.Primary, m.Follower))
	}

	return "Sync stopped: this follower's setup differs from the primary's: " + strings.Join(parts, "; ") + ". Set them the same and restart this follower."
}

func (f *Follower) stopOnConflict(conflict *replicationapi.Conflict, state model.FollowerState) {
	f.setPrimaryVersion(conflict.PrimaryVersion)

	if conflict.Reason == replicationapi.ConflictOtherCluster {
		f.stop(FollowerOtherCluster, fmt.Sprintf("Sync stopped: the primary at %s belongs to another replication cluster than this follower's data. To replace this follower's data with the primary's, run dockvmap -resync on this follower's host.", f.primary), nil)
		return
	}

	f.stop(FollowerDiverged, fmt.Sprintf("Sync stopped: the primary's history doesn't match this follower's (primary at version %d, this follower at %d). Was the primary restored from a backup? To take the primary's data, run dockvmap -resync on this follower's host.", conflict.PrimaryVersion, state.AppliedVersion), nil)
}

func (f *Follower) reportWhileStopped(ctx context.Context) {
	req, _, err := f.request(ctx, false, false)

	if err != nil {
		return
	}

	req.Full = true

	if response, conflict, err := f.client.State(ctx, req, f.currentSync(), f.health(ctx)); err == nil {
		f.contacted()

		if conflict != nil {
			f.setPrimaryVersion(conflict.PrimaryVersion)
		} else if response != nil {
			f.setPrimaryVersion(response.Snapshot.State.Version)
		}
	}
}

// applyState writes the latest snapshot; an internal image keeps its old tag until the new one is in this follower's registry
func (f *Follower) applyState(ctx context.Context) error {
	f.applyMu.Lock()
	defer f.applyMu.Unlock()

	f.mu.Lock()
	latest := f.latest
	f.mu.Unlock()

	if latest == nil {
		return nil
	}

	local, err := f.store.ReplicatedImages(ctx)

	if err != nil {
		return err
	}

	state, err := f.store.FollowerState(ctx)

	if err != nil {
		return err
	}

	snapshot := latest.Snapshot
	held := local

	// images from another cluster are replaced, not moved, so none is held back
	if state.ClusterID != snapshot.State.ClusterID {
		held = nil
	}

	snapshot.Images = f.holdMissingContent(ctx, snapshot, held)

	changed, err := f.store.ApplyReplicationSnapshot(ctx, &snapshot, f.internal)

	if err != nil {
		return err
	}

	if changed > 0 {
		slog.Info("applied the primary's state", "version", snapshot.State.Version, "rows", changed)
		f.notifyTagChanges(local, snapshot.Images)
	}

	return nil
}

func (f *Follower) holdMissingContent(ctx context.Context, snapshot model.ReplicationSnapshot, local []model.ReplicatedImage) []model.ReplicatedImage {
	images := append([]model.ReplicatedImage(nil), snapshot.Images...)

	if f.mirror == nil {
		return images
	}

	internalID := int64(-1)

	for _, registry := range snapshot.Registries {
		if registry.Internal {
			internalID = registry.ID
		}
	}

	current := make(map[int64]model.ReplicatedImage, len(local))

	for _, img := range local {
		current[img.ID] = img
	}

	for i, img := range images {
		old, exists := current[img.ID]

		if img.RegistryID != internalID || !exists || (old.RegistryID == img.RegistryID && old.Repository == img.Repository && old.Tag == img.Tag) {
			continue
		}

		if present, err := f.mirror.HasTag(ctx, img.Repository, img.Tag); err == nil && present {
			continue
		}

		images[i].RegistryID, images[i].Repository, images[i].Tag = old.RegistryID, old.Repository, old.Tag
	}

	return images
}

func (f *Follower) notifyTagChanges(before, after []model.ReplicatedImage) {
	if f.tagsChanged == nil {
		return
	}

	previous := make(map[int64]model.ReplicatedImage, len(before))

	for _, img := range before {
		previous[img.ID] = img
	}

	var names []string

	for _, img := range after {
		old, ok := previous[img.ID]
		delete(previous, img.ID)

		if ok && old == img {
			continue
		}

		names = append(names, img.Name)

		if ok && old.Name != img.Name {
			names = append(names, old.Name)
		}
	}

	for _, removed := range previous {
		names = append(names, removed.Name)
	}

	if len(names) > 0 {
		f.tagsChanged(names...)
	}
}

func (f *Follower) Status(ctx context.Context) (*FollowerStatus, error) {
	state, err := f.store.FollowerState(ctx)

	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	status := &FollowerStatus{
		Primary:             f.primary,
		ClusterID:           state.ClusterID,
		Sync:                f.sync,
		AppliedVersion:      state.AppliedVersion,
		PrimaryVersion:      f.primaryVersion,
		LastAppliedAt:       state.AppliedAt,
		PendingRepositories: f.pending,
		FailedCopies:        f.failed,
		ExtraTags:           f.extraTags,
		InitialCopyDone:     state.InitialCopyDone || f.mirror == nil,
		LastError:           f.lastError,
		CopyError:           f.copyError(),
		Mismatches:          f.mismatches,
	}

	if !f.lastContact.IsZero() {
		contact := f.lastContact
		status.LastContactAt = &contact
	}

	return status, nil
}

func (f *Follower) Ready(ctx context.Context, status *FollowerStatus) (bool, map[string]string) {
	checks := map[string]string{"database": "ok", "setup": "ok"}

	if err := f.store.Ping(ctx); err != nil {
		checks["database"] = err.Error()
	}

	if f.registryPing != nil {
		checks["registry"] = "ok"

		if err := f.registryPing(ctx); err != nil {
			checks["registry"] = err.Error()
		}
	}

	if status.Sync == FollowerSetupMismatch {
		names := make([]string, 0, len(status.Mismatches))

		for _, m := range status.Mismatches {
			names = append(names, m.Setting)
		}

		checks["setup"] = "differs from the primary: " + strings.Join(names, ", ")
	}

	synced := status.ClusterID != "" && status.Sync != FollowerExistingData && status.InitialCopyDone

	return synced && allOK(checks), checks
}

func (f *Follower) health(ctx context.Context) replicationapi.FollowerHealth {
	ctx, cancel := context.WithTimeout(ctx, followerHealthTimeout)
	defer cancel()

	status, err := f.Status(ctx)

	if err != nil {
		return replicationapi.FollowerHealth{ID: f.id, NotReady: "its database can't be read", LastError: err.Error()}
	}

	ready, checks := f.Ready(ctx, status)

	// any other error was about reaching the primary, which this request just did
	if status.Sync == FollowerSyncRunning {
		status.LastError = ""
	}

	return replicationapi.FollowerHealth{
		ID:                  f.id,
		Ready:               ready,
		NotReady:            notReadyReason(ready, status, checks),
		PendingRepositories: status.PendingRepositories,
		FailedCopies:        status.FailedCopies,
		ExtraTags:           status.ExtraTags,
		LastError:           status.LastError,
		CopyError:           status.CopyError,
	}
}

func notReadyReason(ready bool, status *FollowerStatus, checks map[string]string) string {
	if ready {
		return ""
	}

	for _, check := range []string{"database", "registry", "setup"} {
		if result, ok := checks[check]; ok && result != "ok" {
			return "its " + check + " check failed: " + result
		}
	}

	switch {
	case status.Sync == FollowerExistingData:
		return "it has its own data and waits for dockvmap -resync"
	case status.ClusterID == "":
		return "it hasn't synced with this primary yet"
	case !status.InitialCopyDone:
		return "it's still copying this primary's images"
	}

	return "unknown"
}

func (f *Follower) currentSync() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.sync
}

// stopReasons wait for the operator: -resync, or a config fix and a restart
var stopReasons = []string{FollowerDiverged, FollowerOtherCluster, FollowerExistingData, FollowerSetupMismatch}

func (f *Follower) isStopped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Contains(stopReasons, f.sync)
}

func (f *Follower) stop(reason, message string, mismatches []replicationapi.Mismatch) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.sync != reason {
		slog.Warn("sync from the primary stopped", "reason", reason, "detail", message)
	}

	f.sync, f.lastError, f.mismatches = reason, message, mismatches
}

func (f *Follower) resume() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sync, f.lastError, f.mismatches = FollowerSyncRunning, "", nil
}

func (f *Follower) setError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case errors.Is(err, replicationapi.ErrIdentityChanged):
		f.sync = FollowerIdentity
	case errors.Is(err, replicationapi.ErrNewerAPI):
		f.sync = FollowerAPIVersion
	default:
		f.sync = FollowerSyncRunning
	}

	if f.lastError != err.Error() {
		slog.Warn("syncing from the primary failed", "primary", f.primary, "error", err)
	}

	f.lastError = err.Error()
}

func (f *Follower) clearError() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sync, f.lastError = FollowerSyncRunning, ""
}

func (f *Follower) contacted() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastContact = time.Now().UTC()
}

func (f *Follower) setPrimaryVersion(version int64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.primaryVersion = version
}
