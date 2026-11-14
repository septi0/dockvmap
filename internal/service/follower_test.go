package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/store"
)

type fakePrimaryClient struct {
	mu       sync.Mutex
	primary  *store.Store
	response *replicationapi.StateResponse
	conflict *replicationapi.Conflict
	err      error
	requests []StateRequest
	reports  []replicationapi.PullReport
}

func (c *fakePrimaryClient) State(_ context.Context, req StateRequest, _ string) (*replicationapi.StateResponse, *replicationapi.Conflict, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests = append(c.requests, req)

	return c.response, c.conflict, c.err
}

func (c *fakePrimaryClient) Changes(ctx context.Context, repository string, after int64) (*replicationapi.ChangesResponse, error) {
	changes, complete, err := c.primary.RepositoryChanges(ctx, repository, after)

	return &replicationapi.ChangesResponse{Changes: changes, Complete: complete}, err
}

func (c *fakePrimaryClient) ReportPulls(ctx context.Context, report replicationapi.PullReport) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return c.err
	}

	c.reports = append(c.reports, report)

	return NewReplication(ReplicationDeps{Store: c.primary}).RecordPulls(ctx, report)
}

type fakeMirror struct {
	primaryTags map[string][]string
	localTags   map[string][]string
	deleted     []string
	copies      []string
	failCopy    bool
}

func (m *fakeMirror) Repositories(context.Context) ([]string, error) {
	var repositories []string

	for repository := range m.primaryTags {
		repositories = append(repositories, repository)
	}

	return repositories, nil
}

func (m *fakeMirror) Tags(_ context.Context, repository string) ([]string, error) {
	return m.primaryTags[repository], nil
}

func (m *fakeMirror) Copy(_ context.Context, repository, tag string) (bool, error) {
	if m.failCopy {
		return false, errors.New("the primary didn't answer")
	}

	m.copies = append(m.copies, repository+":"+tag)

	if slices.Contains(m.localTags[repository], tag) {
		return false, nil
	}

	m.localTags[repository] = append(m.localTags[repository], tag)

	return true, nil
}

func (m *fakeMirror) HasTag(_ context.Context, repository, tag string) (bool, error) {
	return slices.Contains(m.localTags[repository], tag), nil
}

func (m *fakeMirror) Delete(_ context.Context, repository, reference string) error {
	m.deleted = append(m.deleted, repository+":"+reference)
	m.localTags[repository] = slices.DeleteFunc(m.localTags[repository], func(tag string) bool { return tag == reference })

	return nil
}

func (m *fakeMirror) Ping(context.Context) error                   { return nil }
func (m *fakeMirror) DeletesAllowed(context.Context) (bool, error) { return true, nil }

type replicationPair struct {
	primary  *store.Store
	follower *store.Store
	client   *fakePrimaryClient
	mirror   *fakeMirror
	f        *Follower
	internal int64
}

func newEncryptedStore(t *testing.T) *store.Store {
	t.Helper()

	key := make([]byte, 32)
	rand.Read(key)

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), base64.StdEncoding.EncodeToString(key))

	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}

	t.Cleanup(func() { st.Close() })

	return st
}

func newReplicationPair(t *testing.T) *replicationPair {
	t.Helper()
	ctx := context.Background()

	p := &replicationPair{
		primary:  newEncryptedStore(t),
		follower: newEncryptedStore(t),
		client:   &fakePrimaryClient{},
		mirror:   &fakeMirror{primaryTags: map[string][]string{}, localTags: map[string][]string{}},
	}

	p.client.primary = p.primary

	internalID, err := p.primary.CreateRegistry(ctx, model.Registry{Registry: "registry:5000"})

	if err != nil {
		t.Fatalf("CreateRegistry() error = %v", err)
	}

	p.primary.MarkRegistryInternal(ctx, internalID, "registry:5000", model.RegistryOptions{Insecure: true})
	p.primary.EnsureClusterID(ctx, "cluster-a")
	p.internal = internalID

	p.f = NewFollower(FollowerDeps{
		Store:    p.follower,
		Client:   p.client,
		Setup:    replicationapi.Setup{"virtual_tag": "current"},
		Primary:  "10.0.0.11:9003",
		Mirror:   p.mirror,
		Internal: &model.LocalInternalRegistry{Address: "registry:5000", Options: model.RegistryOptions{Insecure: true}},
	})

	return p
}

func (p *replicationPair) publish(t *testing.T, setup replicationapi.Setup) {
	t.Helper()

	snapshot, err := p.primary.ReplicationSnapshot(context.Background())

	if err != nil {
		t.Fatalf("ReplicationSnapshot() error = %v", err)
	}

	p.client.response = &replicationapi.StateResponse{APIVersion: replicationapi.APIVersion, Setup: setup, Snapshot: *snapshot}
}

func (p *replicationPair) sync(t *testing.T, full, resync bool) {
	t.Helper()

	if err := p.f.syncOnce(context.Background(), full, resync); err != nil {
		t.Fatalf("syncOnce() error = %v", err)
	}
}

func tagOf(t *testing.T, st *store.Store, name string) string {
	t.Helper()

	img, err := st.GetImage(context.Background(), name)

	if err != nil || img == nil {
		t.Fatalf("GetImage(%s) = %v, %v", name, img, err)
	}

	return img.Tag
}

func TestFollowerFirstSyncAppliesThePrimary(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.CreateImage(ctx, &model.Image{Name: "nginx", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	p.publish(t, replicationapi.Setup{"virtual_tag": "current"})

	p.sync(t, true, false)

	if got := tagOf(t, p.follower, "nginx"); got != "1.27" {
		t.Errorf("follower nginx tag = %s, want 1.27", got)
	}

	status, _ := p.f.Status(ctx)

	if status.Sync != FollowerSyncRunning || status.ClusterID != "cluster-a" || status.AppliedVersion == 0 {
		t.Errorf("status = %+v, want running in cluster-a with a version", status)
	}

	if req := p.client.requests[0]; !req.Full || req.HasVersion {
		t.Errorf("first request = %+v, want a full request without a version", req)
	}
}

func TestFollowerStopsOnSetupMismatch(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.CreateImage(ctx, &model.Image{Name: "nginx", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	p.publish(t, replicationapi.Setup{"virtual_tag": "latest"})

	p.sync(t, true, false)

	status, _ := p.f.Status(ctx)

	if status.Sync != FollowerSetupMismatch || len(status.Mismatches) != 1 || status.Mismatches[0].Setting != "virtual_tag" {
		t.Errorf("status = %+v, want stopped on virtual_tag", status)
	}

	if img, _ := p.follower.GetImage(ctx, "nginx"); img != nil {
		t.Errorf("follower applied the state despite the setup mismatch")
	}

	if ready, checks := p.f.Ready(ctx, status); ready || checks["setup"] != "differs from the primary: virtual_tag" {
		t.Errorf("Ready() = %v, %v, want not ready naming virtual_tag", ready, checks)
	}
}

func TestFollowerWithItsOwnDataWaitsForResync(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.follower.CreateProxyToken(ctx, "local", "hash-local", false)
	p.primary.CreateImage(ctx, &model.Image{Name: "nginx", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	p.publish(t, replicationapi.Setup{"virtual_tag": "current"})

	p.sync(t, true, false)

	if status, _ := p.f.Status(ctx); status.Sync != FollowerExistingData {
		t.Fatalf("sync = %s, want %s", status.Sync, FollowerExistingData)
	}

	if token, _ := p.follower.ProxyTokenByHash(ctx, "hash-local"); token == nil {
		t.Fatalf("the follower's own token was removed before a resync")
	}

	p.f.resume()
	p.sync(t, true, true)

	if token, _ := p.follower.ProxyTokenByHash(ctx, "hash-local"); token != nil {
		t.Errorf("the follower's own token survived the resync")
	}

	if got := tagOf(t, p.follower, "nginx"); got != "1.27" {
		t.Errorf("follower nginx tag after resync = %s, want 1.27", got)
	}
}

func TestFollowerStopsWhenThePrimaryDiverged(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.CreateImage(ctx, &model.Image{Name: "nginx", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	p.publish(t, replicationapi.Setup{})
	p.sync(t, true, false)
	p.f.copyRepositories(ctx)

	p.client.response, p.client.conflict = nil, &replicationapi.Conflict{Reason: replicationapi.ConflictDiverged, PrimaryVersion: 1}
	p.sync(t, false, false)

	status, _ := p.f.Status(ctx)

	if status.Sync != FollowerDiverged || status.PrimaryVersion != 1 {
		t.Errorf("status = %+v, want stopped as diverged with the primary at 1", status)
	}

	if req := p.client.requests[1]; !req.HasVersion || req.Cluster != "cluster-a" || req.Version != status.AppliedVersion {
		t.Errorf("second request = %+v, want the applied version and cluster", req)
	}

	if ready, _ := p.f.Ready(ctx, status); !ready {
		t.Errorf("a diverged follower with good data isn't ready, want it ready so failover still works")
	}
}

func TestFollowerHoldsATagUntilItsContentIsCopied(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.CreateImage(ctx, &model.Image{Name: "web", RegistryID: p.internal, Repository: "team/web", Tag: "v1"})
	p.primary.RecordRepositoryChange(ctx, "team/web", "v1", false)
	p.mirror.primaryTags["team/web"] = []string{"v1"}
	p.mirror.localTags["team/web"] = []string{"v1"}
	p.publish(t, replicationapi.Setup{})
	p.sync(t, true, false)
	p.f.copyRepositories(ctx)

	web, _ := p.primary.GetImage(ctx, "web")
	p.primary.UpdateImageTag(ctx, nil, web.ID, "v2")
	p.primary.RecordRepositoryChange(ctx, "team/web", "v2", false)
	p.mirror.primaryTags["team/web"] = []string{"v1", "v2"}
	p.publish(t, replicationapi.Setup{})
	p.sync(t, false, false)

	if got := tagOf(t, p.follower, "web"); got != "v1" {
		t.Errorf("web tag before v2 is copied = %s, want v1 held", got)
	}

	if failed := p.f.copyRepositories(ctx); failed != 0 {
		t.Fatalf("copyRepositories() failed %d", failed)
	}

	if got := tagOf(t, p.follower, "web"); got != "v2" {
		t.Errorf("web tag after the copy = %s, want v2", got)
	}

	if state, _ := p.follower.FollowerState(ctx); !state.InitialCopyDone {
		t.Errorf("initial copy isn't marked done after a clean pass")
	}
}

func TestFollowerReplaysOnlyWhatChanged(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.RecordRepositoryChange(ctx, "team/web", "v1", false)
	p.mirror.primaryTags["team/web"] = []string{"v1", "v2", "v3"}
	p.publish(t, replicationapi.Setup{})
	p.sync(t, true, false)
	p.f.copyRepositories(ctx)

	if len(p.mirror.copies) != 3 {
		t.Fatalf("first copy = %v, want every tag of a repository never copied before", p.mirror.copies)
	}

	p.mirror.copies = nil
	p.mirror.localTags["team/api"] = []string{"old"}
	p.mirror.primaryTags["team/web"] = []string{"v2", "v3", "v4"}
	p.primary.RecordRepositoryChange(ctx, "team/web", "v4", false)
	p.primary.RecordRepositoryChange(ctx, "team/web", "v1", true)
	p.publish(t, replicationapi.Setup{})
	p.sync(t, false, false)
	p.f.copyRepositories(ctx)

	if !slices.Equal(p.mirror.copies, []string{"team/web:v4"}) || !slices.Equal(p.mirror.deleted, []string{"team/web:v1"}) {
		t.Errorf("copies %v, deletes %v, want only the pushed v4 and the recorded delete of v1", p.mirror.copies, p.mirror.deleted)
	}

	if !slices.Equal(p.mirror.localTags["team/api"], []string{"old"}) {
		t.Errorf("team/api, missing on the primary without a recorded delete, = %v, want it kept", p.mirror.localTags["team/api"])
	}

	p.f.copyRepositories(ctx)

	if len(p.mirror.deleted) != 1 || len(p.mirror.copies) != 1 {
		t.Errorf("changes were replayed again: copies %v, deletes %v", p.mirror.copies, p.mirror.deleted)
	}
}

func TestFollowerComparesTheWholeRepositoryWhenChangesWerePruned(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.RecordRepositoryChange(ctx, "team/web", "v1", false)
	p.mirror.primaryTags["team/web"] = []string{"v1"}
	p.publish(t, replicationapi.Setup{})
	p.sync(t, true, false)
	p.f.copyRepositories(ctx)

	p.primary.RecordRepositoryChange(ctx, "team/web", "v2", false)
	p.primary.CleanupReplicationChanges(ctx, time.Now().Add(time.Hour))
	p.mirror.primaryTags["team/web"] = []string{"v1", "v2"}
	p.mirror.localTags["team/web"] = append(p.mirror.localTags["team/web"], "kept")
	p.publish(t, replicationapi.Setup{})
	p.sync(t, false, false)
	p.f.copyRepositories(ctx)

	if !slices.Contains(p.mirror.localTags["team/web"], "v2") || !slices.Contains(p.mirror.localTags["team/web"], "kept") || len(p.mirror.deleted) != 0 {
		t.Errorf("follower tags = %v, deletes %v, want v2 copied and nothing deleted", p.mirror.localTags["team/web"], p.mirror.deleted)
	}
}

func TestFollowerCopyFailureKeepsTheRepositoryPending(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.RecordRepositoryChange(ctx, "team/web", "v1", false)
	p.mirror.primaryTags["team/web"] = []string{"v1"}
	p.mirror.failCopy = true
	p.publish(t, replicationapi.Setup{})
	p.sync(t, true, false)

	if failed := p.f.copyRepositories(ctx); failed != 1 {
		t.Errorf("copyRepositories() failed = %d, want 1", failed)
	}

	status, _ := p.f.Status(ctx)

	if status.PendingRepositories != 1 || status.FailedCopies != 1 || status.InitialCopyDone {
		t.Errorf("status = %+v, want team/web pending and the initial copy not done", status)
	}

	if ready, _ := p.f.Ready(ctx, status); ready {
		t.Errorf("a follower whose first copy failed is ready, want not ready")
	}
}

func TestFollowerRetriesCopiesWhenThePrimaryAnswersAgain(t *testing.T) {
	p := newReplicationPair(t)
	p.publish(t, replicationapi.Setup{})
	p.client.response, p.client.err = nil, errors.New("the primary at 10.0.0.11:9003 can't be reached")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go p.f.run(ctx)

	time.Sleep(200 * time.Millisecond)

	if len(p.f.copies) != 0 {
		t.Fatalf("copies queued while the primary is unreachable, want none")
	}

	p.client.mu.Lock()
	p.client.err = nil
	p.client.mu.Unlock()
	time.Sleep(1500 * time.Millisecond)

	if len(p.f.copies) != 1 {
		t.Errorf("no copy queued after the primary answered again, want pending copies retried at once")
	}
}

func TestFollowerIsReadyFollowingAnUnchangedUpgradedPrimary(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.publish(t, replicationapi.Setup{"virtual_tag": "current"})
	p.client.response.Snapshot.State.Version, p.client.response.Snapshot.State.Tag = 0, ""

	p.sync(t, true, false)
	p.f.copyRepositories(ctx)

	status, _ := p.f.Status(ctx)

	if ready, checks := p.f.Ready(ctx, status); !ready {
		t.Errorf("Ready() = false, %v, want ready after syncing a primary still at version 0", checks)
	}
}

func TestFollowerFirstCopyIncludesImagesPushedBeforeReplication(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.mirror.primaryTags["team/legacy"] = []string{"v1", "v2"}
	p.publish(t, replicationapi.Setup{"virtual_tag": "current"})

	p.sync(t, true, false)

	if failed := p.f.copyRepositories(ctx); failed != 0 {
		t.Fatalf("copyRepositories() failed %d repositories", failed)
	}

	if got := p.mirror.localTags["team/legacy"]; !slices.Equal(got, []string{"v1", "v2"}) {
		t.Errorf("follower team/legacy = %v, want v1 and v2 copied by the first copy though no push was recorded", got)
	}

	p.mirror.primaryTags["team/later"] = []string{"v1"}
	p.mirror.copies = nil
	p.f.copyRepositories(ctx)

	if len(p.mirror.copies) != 0 {
		t.Errorf("after the first copy: copied %v, want unrecorded repositories left to the reconcile", p.mirror.copies)
	}
}

func TestFollowerReportsPullsUntilThePrimaryTakesThem(t *testing.T) {
	ctx := context.Background()
	p := newReplicationPair(t)
	p.primary.CreateImage(ctx, &model.Image{Name: "web", RegistryID: p.internal, Repository: "team/web", Tag: "v1"})
	tokenID, _ := p.primary.CreateProxyToken(ctx, "hosts", "hash-hosts", false)
	p.publish(t, replicationapi.Setup{"virtual_tag": "current"})
	p.sync(t, true, false)

	web, _ := p.follower.GetImage(ctx, "web")
	pulledAt := time.Now().UTC().Truncate(time.Second)
	p.follower.RecordProxyUsage(ctx, []model.ImageUsage{{ImageID: web.ID, PulledAt: pulledAt, PulledTag: "v1"}}, []model.ProxyTokenUsage{{TokenID: tokenID, UsedAt: pulledAt}})

	reporter := &pullReporter{images: map[int64]time.Time{}, tokens: map[int64]time.Time{}}
	p.client.err = errors.New("the primary is down")
	p.f.reportPulls(ctx, reporter)
	p.client.err = nil
	p.f.reportPulls(ctx, reporter)

	got, _ := p.primary.GetImage(ctx, "web")

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(pulledAt) || got.LastPulledTag == nil || *got.LastPulledTag != "v1" {
		t.Errorf("primary web last pulled = %v %v, want %v on v1 once the primary answers again", got.LastPulledAt, got.LastPulledTag, pulledAt)
	}

	if token, _ := p.primary.GetProxyTokenByID(ctx, tokenID); token == nil || token.LastUsedAt == nil || !token.LastUsedAt.Equal(pulledAt) {
		t.Errorf("primary token last used = %+v, want %v", token, pulledAt)
	}

	p.f.reportPulls(ctx, reporter)

	if len(p.client.reports) != 1 {
		t.Errorf("reports sent = %d, want 1: nothing changed after the first one the primary took", len(p.client.reports))
	}

	p.primary.RecordProxyUsage(ctx, []model.ImageUsage{{ImageID: web.ID, PulledAt: pulledAt.Add(time.Hour), PulledTag: "v2"}}, nil)
	p.follower.RecordProxyUsage(ctx, []model.ImageUsage{{ImageID: web.ID, PulledAt: pulledAt.Add(time.Minute), PulledTag: "v1"}}, nil)
	p.f.reportPulls(ctx, reporter)

	if got, _ := p.primary.GetImage(ctx, "web"); *got.LastPulledTag != "v2" {
		t.Errorf("primary web last pulled tag = %s, want v2: an older pull on the follower must not move it back", *got.LastPulledTag)
	}
}
