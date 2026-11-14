package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func newReplicationTestStore(t *testing.T) *Store {
	t.Helper()

	key := make([]byte, 32)
	rand.Read(key)

	s, err := New(filepath.Join(t.TempDir(), "test.db"), base64.StdEncoding.EncodeToString(key))

	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { s.Close() })

	return s
}

func version(t *testing.T, s *Store) int64 {
	t.Helper()

	state, err := s.ReplicationState(context.Background())

	if err != nil {
		t.Fatalf("ReplicationState() error = %v", err)
	}

	return state.Version
}

func TestReplicatedTablesHaveTriggers(t *testing.T) {
	s := newReplicationTestStore(t)

	for _, table := range replicatedTables {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			var count int

			err := s.db.QueryRow(`
				SELECT COUNT(*) FROM sqlite_master
				WHERE type = 'trigger' AND tbl_name = ? AND sql LIKE '%AFTER ' || ? || '%' AND sql LIKE '%UPDATE replication_state%'
			`, table, event).Scan(&count)

			if err != nil {
				t.Fatalf("reading triggers: %v", err)
			}

			if count != 1 {
				t.Errorf("table %s has %d %s triggers bumping the replication version, want 1", table, count, event)
			}
		}
	}
}

func TestVersionBumpsOnlyWhenReplicatedDataChanges(t *testing.T) {
	ctx := context.Background()
	s := newReplicationTestStore(t)

	img := &model.Image{Name: "web", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"}

	if err := s.CreateImage(ctx, img); err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	stored, _ := s.GetImage(ctx, "web")
	before := version(t, s)

	if _, err := s.UpdateImageCheck(ctx, nil, stored.ID, nil, time.Now(), "hash"); err != nil {
		t.Fatalf("UpdateImageCheck() error = %v", err)
	}

	if _, err := s.UpdateImageTag(ctx, nil, stored.ID, "1.27"); err != nil {
		t.Fatalf("UpdateImageTag() error = %v", err)
	}

	if got := version(t, s); got != before {
		t.Errorf("version after a tag check and an unchanged tag = %d, want %d", got, before)
	}

	if _, err := s.UpdateImageTag(ctx, nil, stored.ID, "1.28"); err != nil {
		t.Fatalf("UpdateImageTag() error = %v", err)
	}

	if got := version(t, s); got != before+1 {
		t.Errorf("version after a tag change = %d, want %d", got, before+1)
	}

	if err := s.RecordRepositoryChange(ctx, "team/web", "v1", false); err != nil {
		t.Fatalf("RecordRepositoryChange() error = %v", err)
	}

	if got := version(t, s); got != before+2 {
		t.Errorf("version after a push = %d, want %d", got, before+2)
	}
}

func TestRepositoryChanges(t *testing.T) {
	ctx := context.Background()
	s := newReplicationTestStore(t)

	for _, change := range []struct {
		reference string
		deleted   bool
	}{{"v1", false}, {"v2", false}, {"v1", true}} {
		if err := s.RecordRepositoryChange(ctx, "team/web", change.reference, change.deleted); err != nil {
			t.Fatalf("RecordRepositoryChange() error = %v", err)
		}
	}

	snapshot, _ := s.ReplicationSnapshot(ctx)

	if len(snapshot.Repositories) != 1 || snapshot.Repositories[0].Counter != 3 {
		t.Errorf("repositories = %+v, want team/web at 3", snapshot.Repositories)
	}

	changes, complete, err := s.RepositoryChanges(ctx, "team/web", 1)

	if err != nil || !complete || len(changes) != 2 || changes[0].Reference != "v2" || !changes[1].Deleted {
		t.Errorf("RepositoryChanges(after 1) = %+v, %v, %v, want push v2 then delete v1", changes, complete, err)
	}

	if _, complete, _ := s.RepositoryChanges(ctx, "team/web", 5); complete {
		t.Errorf("changes after a counter ahead of the primary's are complete, want a whole-repository compare")
	}

	if removed, err := s.CleanupReplicationChanges(ctx, time.Now().Add(time.Hour)); err != nil || removed != 3 {
		t.Errorf("CleanupReplicationChanges() = %d, %v, want 3 removed", removed, err)
	}

	if _, complete, _ := s.RepositoryChanges(ctx, "team/web", 1); complete {
		t.Errorf("changes after pruning are complete, want a whole-repository compare")
	}

	if changes, complete, _ := s.RepositoryChanges(ctx, "team/web", 3); !complete || len(changes) != 0 {
		t.Errorf("changes of a follower already at the counter = %+v, %v, want none and complete", changes, complete)
	}
}

func TestSnapshotApplyMirrorsThePrimary(t *testing.T) {
	ctx := context.Background()
	primary := newReplicationTestStore(t)
	follower := newReplicationTestStore(t)

	if _, err := follower.CreateRegistry(ctx, model.Registry{Registry: "registry:5000"}); err != nil {
		t.Fatalf("CreateRegistry() error = %v", err)
	}

	followerInternal, _ := follower.GetRegistryInfoByHost(ctx, "registry:5000")

	if err := follower.MarkRegistryInternal(ctx, followerInternal.ID, "registry:5000", model.RegistryOptions{Insecure: true}); err != nil {
		t.Fatalf("MarkRegistryInternal() error = %v", err)
	}

	quayID, err := primary.CreateRegistry(ctx, model.Registry{Registry: "quay.io", Username: "bot", Credential: "s3cret"})

	if err != nil {
		t.Fatalf("CreateRegistry() error = %v", err)
	}

	internalID, _ := primary.CreateRegistry(ctx, model.Registry{Registry: "images:5000"})

	if err := primary.MarkRegistryInternal(ctx, internalID, "images:5000", model.RegistryOptions{Insecure: true}); err != nil {
		t.Fatalf("MarkRegistryInternal() error = %v", err)
	}

	groupID, _ := primary.CreateImageGroup(ctx, nil, "immich")

	for _, img := range []*model.Image{
		{Name: "server", RegistryID: quayID, Repository: "immich/server", Tag: "v2"},
		{Name: "web", RegistryID: internalID, Repository: "team/web", Tag: "v1"},
	} {
		if err := primary.CreateImage(ctx, img); err != nil {
			t.Fatalf("CreateImage() error = %v", err)
		}
	}

	server, _ := primary.GetImage(ctx, "server")
	primary.UpdateImageGroupMembership(ctx, nil, server.ID, &groupID)
	primary.UpdateImagePinned(ctx, nil, server.ID, true)
	primary.CreateProxyToken(ctx, "ci", "hash-ci", true)

	local := &model.LocalInternalRegistry{Address: "registry:5000", Options: model.RegistryOptions{Insecure: true}}

	snapshot, err := primary.ReplicationSnapshot(ctx)

	if err != nil {
		t.Fatalf("ReplicationSnapshot() error = %v", err)
	}

	if _, err := follower.ApplyReplicationSnapshot(ctx, snapshot, local); err != nil {
		t.Fatalf("ApplyReplicationSnapshot() error = %v", err)
	}

	got, _ := follower.GetImage(ctx, "server")

	if got == nil || got.ID != server.ID || got.Tag != "v2" || !got.Pinned || got.GroupID == nil || *got.GroupID != groupID {
		t.Fatalf("follower image = %+v, want server on v2, pinned, in group %d", got, groupID)
	}

	credentials, _ := follower.GetRegistryCredentials(ctx, "quay.io")

	if credentials == nil || credentials.Username != "bot" || credentials.Credential != "s3cret" {
		t.Errorf("follower quay.io login = %+v, want bot / s3cret re-encrypted with the follower's key", credentials)
	}

	internal, _ := follower.GetInternalRegistry(ctx)

	if internal == nil || internal.ID != internalID || internal.Registry != "registry:5000" {
		t.Errorf("follower internal registry = %+v, want id %d at the follower's own address", internal, internalID)
	}

	if token, _ := follower.ProxyTokenByHash(ctx, "hash-ci"); token == nil || !token.CanPush {
		t.Errorf("follower proxy token = %+v, want ci with can push", token)
	}

	if changed, err := follower.ApplyReplicationSnapshot(ctx, snapshot, local); err != nil || changed != 0 {
		t.Errorf("applying the same snapshot again changed %d rows (err %v), want 0", changed, err)
	}

	follower.UpdateImageDigest(ctx, got.ID, "v2", nil, time.Now())
	primary.UpdateImageTag(ctx, nil, server.ID, "v3")
	web, _ := primary.GetImage(ctx, "web")
	primary.DeleteImage(ctx, web.ID)

	snapshot, _ = primary.ReplicationSnapshot(ctx)

	if changed, err := follower.ApplyReplicationSnapshot(ctx, snapshot, local); err != nil || changed != 2 {
		t.Errorf("applying a tag change and a delete changed %d rows (err %v), want 2", changed, err)
	}

	if got, _ := follower.GetImage(ctx, "server"); got == nil || got.Tag != "v3" {
		t.Errorf("follower server after the tag change = %+v, want v3", got)
	}

	if got, _ := follower.GetImage(ctx, "web"); got != nil {
		t.Errorf("follower still has web after it was deleted on the primary")
	}

	state, _ := follower.FollowerState(ctx)

	if state.AppliedVersion != snapshot.State.Version || state.AppliedTag != snapshot.State.Tag {
		t.Errorf("follower state = %+v, want version %d tag %s", state, snapshot.State.Version, snapshot.State.Tag)
	}
}

func TestSnapshotApplyReplacesAFollowersOwnData(t *testing.T) {
	ctx := context.Background()
	primary := newReplicationTestStore(t)
	follower := newReplicationTestStore(t)

	ownID, _ := follower.CreateRegistry(ctx, model.Registry{Registry: "old.example:5000"})
	follower.CreateImage(ctx, &model.Image{Name: "legacy", RegistryID: ownID, Repository: "team/legacy", Tag: "v1"})
	follower.CreateImage(ctx, &model.Image{Name: "web", RegistryID: 1, Repository: "library/nginx", Tag: "1.26"})

	primary.EnsureClusterID(ctx, "c1")
	primary.CreateImage(ctx, &model.Image{Name: "web", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	primary.CreateImage(ctx, &model.Image{Name: "api", RegistryID: 1, Repository: "library/httpd", Tag: "2.4"})
	snapshot, _ := primary.ReplicationSnapshot(ctx)

	if _, err := follower.ApplyReplicationSnapshot(ctx, snapshot, nil); err != nil {
		t.Fatalf("ApplyReplicationSnapshot() error = %v, want the follower's own registry and images replaced", err)
	}

	if got, _ := follower.GetImage(ctx, "web"); got == nil || got.Tag != "1.27" {
		t.Errorf("follower web = %+v, want the primary's on 1.27", got)
	}

	if own, _ := follower.GetRegistryInfoByHost(ctx, "old.example:5000"); own != nil {
		t.Errorf("follower still has its own registry %+v", own)
	}
}

func TestFollowerHasLocalData(t *testing.T) {
	ctx := context.Background()
	s := newReplicationTestStore(t)

	if has, err := s.FollowerHasLocalData(ctx); err != nil || has {
		t.Errorf("fresh install has local data = %v (err %v), want false", has, err)
	}

	if _, err := s.CreateProxyToken(ctx, "host", "hash", false); err != nil {
		t.Fatalf("CreateProxyToken() error = %v", err)
	}

	if has, _ := s.FollowerHasLocalData(ctx); !has {
		t.Errorf("install with a proxy token has local data = false, want true")
	}
}

func TestResyncRequestIsTakenOnce(t *testing.T) {
	ctx := context.Background()
	s := newReplicationTestStore(t)
	s.SetFollowerRepository(ctx, "team/web", 4)
	s.MarkInitialCopyDone(ctx)

	if err := s.RequestResync(ctx); err != nil {
		t.Fatalf("RequestResync() error = %v", err)
	}

	if taken, _ := s.TakeResyncRequest(ctx); !taken {
		t.Errorf("first TakeResyncRequest() = false, want true")
	}

	if taken, _ := s.TakeResyncRequest(ctx); taken {
		t.Errorf("second TakeResyncRequest() = true, want false")
	}

	copied, _ := s.FollowerRepositories(ctx)
	state, _ := s.FollowerState(ctx)

	if len(copied) != 0 || state.InitialCopyDone {
		t.Errorf("after a resync request: copied %v, initial copy done %v, want a full compare again", copied, state.InitialCopyDone)
	}
}
