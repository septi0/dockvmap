package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/store"
)

func newTestReplication(t *testing.T) (*Replication, *store.Store) {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}

	t.Cleanup(func() { st.Close() })

	r := NewReplication(ReplicationDeps{
		Store:       st,
		Audit:       NewAudit(st),
		Setup:       replicationapi.Setup{"virtual_tag": "current"},
		Fingerprint: "sha256:" + strings.Repeat("ab", 32),
	})
	r.waitTimeout = 300 * time.Millisecond
	r.watchInterval = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	return r, st
}

func currentState(t *testing.T, st *store.Store) model.ReplicationState {
	t.Helper()

	state, err := st.ReplicationState(context.Background())

	if err != nil {
		t.Fatalf("ReplicationState() error = %v", err)
	}

	return state
}

func TestReplicationStateAnswersAtOnceWhenTheFollowerIsBehind(t *testing.T) {
	r, st := newTestReplication(t)
	st.RecordRepositoryChange(context.Background(), "team/web", "v1", false)
	state := currentState(t, st)

	started := time.Now()
	response, conflict, err := r.State(context.Background(), StateRequest{Version: state.Version - 1, HasVersion: true, Cluster: state.ClusterID})

	if err != nil || conflict != nil || response == nil {
		t.Fatalf("State() = %v, %v, %v, want a snapshot", response, conflict, err)
	}

	if time.Since(started) > 100*time.Millisecond {
		t.Errorf("State() took %v for a follower that is behind, want an immediate answer", time.Since(started))
	}

	if response.Snapshot.State.Version != state.Version || response.Setup["virtual_tag"] != "current" || response.APIVersion != replicationapi.APIVersion {
		t.Errorf("response = %+v, want version %d with the setup", response, state.Version)
	}
}

func TestReplicationStateWaitsForAChange(t *testing.T) {
	r, st := newTestReplication(t)
	state := currentState(t, st)

	go func() {
		time.Sleep(50 * time.Millisecond)
		st.RecordRepositoryChange(context.Background(), "team/web", "v1", false)
	}()

	response, conflict, err := r.State(context.Background(), StateRequest{Version: state.Version, Tag: state.Tag, HasVersion: true})

	if err != nil || conflict != nil || response == nil || response.Snapshot.State.Version == state.Version {
		t.Fatalf("State() = %v, %v, %v, want the snapshot after the change", response, conflict, err)
	}
}

func TestReplicationStateAnswersNothingChanged(t *testing.T) {
	r, st := newTestReplication(t)
	state := currentState(t, st)

	response, conflict, err := r.State(context.Background(), StateRequest{Version: state.Version, Tag: state.Tag, HasVersion: true})

	if err != nil || conflict != nil || response != nil {
		t.Errorf("State() = %v, %v, %v, want nothing changed after the wait", response, conflict, err)
	}

	response, _, _ = r.State(context.Background(), StateRequest{Version: state.Version, Tag: state.Tag, HasVersion: true, Full: true})

	if response == nil {
		t.Errorf("State() with full = nil, want the snapshot without waiting")
	}
}

func TestReplicationStateConflicts(t *testing.T) {
	r, st := newTestReplication(t)
	st.RecordRepositoryChange(context.Background(), "team/web", "v1", false)
	state := currentState(t, st)

	tests := []struct {
		name string
		req  StateRequest
		want string
	}{
		{"follower ahead", StateRequest{Version: state.Version + 5, Tag: "x", HasVersion: true}, replicationapi.ConflictDiverged},
		{"same version, other history", StateRequest{Version: state.Version, Tag: "other", HasVersion: true}, replicationapi.ConflictDiverged},
		{"other cluster", StateRequest{Version: 1, HasVersion: true, Cluster: "another"}, replicationapi.ConflictOtherCluster},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, conflict, err := r.State(context.Background(), tt.req)

			if err != nil || conflict == nil || conflict.Reason != tt.want || conflict.PrimaryVersion != state.Version {
				t.Errorf("State() conflict = %+v, %v, want %s at version %d", conflict, err, tt.want, state.Version)
			}
		})
	}
}

func TestReplicationTokens(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestReplication(t)

	id, token, err := r.CreateToken(ctx, " standby-1 ")

	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	fingerprint, secret, err := replicationapi.ParseToken(token)

	if err != nil || fingerprint != r.fingerprint {
		t.Fatalf("token = %q (%v), want one carrying the primary's fingerprint", token, err)
	}

	matched, err := r.Authenticate(ctx, secret)

	if err != nil || matched == nil || matched.ID != id || matched.Label != "standby-1" {
		t.Errorf("Authenticate() = %+v, %v, want standby-1", matched, err)
	}

	if matched, _ := r.Authenticate(ctx, token); matched != nil {
		t.Errorf("Authenticate() with the whole token = %+v, want nil: only the secret is sent", matched)
	}

	if _, _, err := r.CreateToken(ctx, "  "); err == nil {
		t.Errorf("CreateToken() with a blank label succeeded, want an error")
	}

	if deleted, err := r.DeleteToken(ctx, id); err != nil || !deleted {
		t.Errorf("DeleteToken() = %v, %v, want deleted", deleted, err)
	}

	if matched, _ := r.Authenticate(ctx, secret); matched != nil {
		t.Errorf("Authenticate() after delete = %+v, want nil", matched)
	}
}

func TestRecordPullsDropsWhatCantBeATag(t *testing.T) {
	ctx := context.Background()
	r, st := newTestReplication(t)
	st.CreateImage(ctx, &model.Image{Name: "web", RegistryID: 1, Repository: "library/nginx", Tag: "1.27"})
	web, _ := st.GetImage(ctx, "web")
	at := time.Now().UTC().Truncate(time.Second)

	r.RecordPulls(ctx, replicationapi.PullReport{Images: []model.ImageUsage{{ImageID: web.ID, PulledAt: at, PulledTag: "<script>"}}})

	if got, _ := st.GetImage(ctx, "web"); got.LastPulledAt != nil {
		t.Errorf("last pulled = %v after a report with an invalid tag, want it ignored", got.LastPulledAt)
	}

	r.RecordPulls(ctx, replicationapi.PullReport{Images: []model.ImageUsage{{ImageID: web.ID, PulledAt: at, PulledTag: "1.27"}}})

	if got, _ := st.GetImage(ctx, "web"); got.LastPulledAt == nil || *got.LastPulledTag != "1.27" {
		t.Errorf("last pulled = %v %v, want the valid report recorded", got.LastPulledAt, got.LastPulledTag)
	}
}
