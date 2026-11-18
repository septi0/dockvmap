package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const (
	followerA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	followerB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func newTestFollowerToken(t *testing.T, r *Replication, label string) *model.ReplicationToken {
	t.Helper()

	id, _, err := r.CreateToken(context.Background(), label)

	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}

	return &model.ReplicationToken{ID: id, Label: label}
}

func healthOf(id string) *replicationapi.FollowerHealth {
	return &replicationapi.FollowerHealth{ID: id, Ready: true}
}

func listFollowers(t *testing.T, r *Replication) []SeenFollower {
	t.Helper()

	followers, err := r.Followers(context.Background())

	if err != nil {
		t.Fatalf("Followers() error = %v", err)
	}

	return followers
}

func onlyFollower(t *testing.T, r *Replication) SeenFollower {
	t.Helper()

	followers := listFollowers(t, r)

	if len(followers) != 1 {
		t.Fatalf("Followers() = %+v, want one follower", followers)
	}

	return followers[0]
}

func TestFollowersAreNamedAndKeptByWhatTheySend(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestReplication(t)
	token := newTestFollowerToken(t, r, "token-1")

	r.SeeFollower(ctx, token, "10.0.0.12", "1.14.0", FollowerSyncRunning, 7, &replicationapi.FollowerHealth{ID: followerA, Ready: true, FailedCopies: 2})

	f := onlyFollower(t, r)

	if f.Name != "" || f.Address != "10.0.0.12" || f.Token != "token-1" || f.AppliedVersion != 7 || f.Connection != FollowerConnected {
		t.Errorf("follower = %+v, want 10.0.0.12 on token-1, connected at version 7", f)
	}

	if f.Health == nil || f.Health.FailedCopies != 2 || !f.HasProblem() {
		t.Errorf("follower health = %+v, problem %v, want 2 failed copies counted as a problem", f.Health, f.HasProblem())
	}

	other := newTestFollowerToken(t, r, "token-2")
	r.SeeFollower(ctx, other, "10.0.0.20", "1.14.0", FollowerSyncRunning, 8, &replicationapi.FollowerHealth{ID: followerA, Ready: true, PendingRepositories: 3})

	if f := onlyFollower(t, r); f.HasProblem() || f.Address != "10.0.0.20" || f.Token != "token-2" {
		t.Errorf("follower with a new address and token = %+v, want the same row updated and no problem", f)
	}
}

func TestFollowersTooOldToSendAnIDAreKeptByAddress(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestReplication(t)
	first := newTestFollowerToken(t, r, "token-1")
	second := newTestFollowerToken(t, r, "token-2")
	r.SeeFollower(ctx, first, "10.0.0.12", "1.13.0", FollowerSyncRunning, 1, nil)
	r.SeeFollower(ctx, second, "10.0.0.12", "1.13.0", FollowerSyncRunning, 1, nil)
	r.SeeFollower(ctx, first, "10.0.0.13", "1.13.0", FollowerSyncRunning, 1, &replicationapi.FollowerHealth{ID: "not-an-id"})

	followers := listFollowers(t, r)

	if len(followers) != 2 || followers[0].Address != "10.0.0.12" || followers[1].Address != "10.0.0.13" {
		t.Errorf("Followers() = %+v, want one row per address, named by it, whatever token they use", followers)
	}
}

func TestFollowersAfterAPrimaryRestart(t *testing.T) {
	ctx := context.Background()
	r, st := newTestReplication(t)
	token := newTestFollowerToken(t, r, "token-1")
	st.SaveReplicationFollower(ctx, model.ReplicationFollower{FollowerID: followerA, Address: "10.0.0.12", TokenID: token.ID, Version: "1.14.0", LastSeenAt: time.Now()})

	if f := onlyFollower(t, r); f.Connection != FollowerWaiting || f.HasProblem() || f.AppliedVersion != 0 {
		t.Errorf("follower saved a moment ago but not seen since this start = %+v, want waiting and no problem", f)
	}

	r.startedAt = time.Now().Add(-time.Hour)

	if f := onlyFollower(t, r); f.Connection != FollowerMissing || !f.HasProblem() || f.Version != "1.14.0" {
		t.Errorf("follower unseen for an hour = %+v, want missing with its saved version", f)
	}

	r.SeeFollower(ctx, token, "10.0.0.12", "1.14.0", "diverged", 3, healthOf(followerA))

	if f := onlyFollower(t, r); f.Connection != FollowerConnected || !f.HasProblem() {
		t.Errorf("stopped follower = %+v, want connected and a problem", f)
	}
}

func TestFollowersSharingATokenAndOutlivingIt(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestReplication(t)
	shared := newTestFollowerToken(t, r, "shared")
	r.SeeFollower(ctx, shared, "10.0.0.12", "1.14.0", FollowerSyncRunning, 1, healthOf(followerA))
	r.SeeFollower(ctx, shared, "10.0.0.13", "1.14.0", FollowerSyncRunning, 1, healthOf(followerB))

	followers := listFollowers(t, r)

	if len(followers) != 2 || followers[0].Address != "10.0.0.12" || followers[1].Address != "10.0.0.13" {
		t.Fatalf("Followers() = %+v, want both followers on the shared token listed apart", followers)
	}

	if _, err := r.DeleteToken(ctx, shared.ID); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}

	followers = listFollowers(t, r)

	if len(followers) != 2 || followers[0].Token != "" || followers[1].Token != "" {
		t.Errorf("Followers() after deleting their token = %+v, want both kept with no token", followers)
	}

	if removed, err := r.RemoveFollower(ctx, followers[0].ID); err != nil || !removed {
		t.Fatalf("RemoveFollower() = %v, %v", removed, err)
	}

	if f := onlyFollower(t, r); f.Address != "10.0.0.13" {
		t.Errorf("left = %+v, want only the follower at 10.0.0.13", f)
	}
}

func TestFollowersAreRenamedOnThePrimaryOnly(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestReplication(t)
	token := newTestFollowerToken(t, r, "token-1")
	r.SeeFollower(ctx, token, "10.0.0.12", "1.14.0", FollowerSyncRunning, 1, healthOf(followerA))
	id := onlyFollower(t, r).ID

	if renamed, err := r.RenameFollower(ctx, id, "  DC standby  "); err != nil || !renamed {
		t.Fatalf("RenameFollower() = %v, %v", renamed, err)
	}

	r.SeeFollower(ctx, token, "10.0.0.20", "1.14.1", FollowerSyncRunning, 2, healthOf(followerA))

	if f := onlyFollower(t, r); f.Name != "DC standby" || f.Address != "10.0.0.20" {
		t.Errorf("renamed follower after a new address = %+v, want the name kept", f)
	}

	if _, err := r.RenameFollower(ctx, id, strings.Repeat("x", 101)); !errors.Is(err, ErrInvalidFollowerName) {
		t.Errorf("RenameFollower() with 101 characters error = %v, want ErrInvalidFollowerName", err)
	}

	if renamed, _ := r.RenameFollower(ctx, id, ""); !renamed || onlyFollower(t, r).Name != "" {
		t.Errorf("clearing the name: follower = %+v, want no name", onlyFollower(t, r))
	}

	if renamed, err := r.RenameFollower(ctx, 999, "x"); err != nil || renamed {
		t.Errorf("RenameFollower() of a missing follower = %v, %v, want not found", renamed, err)
	}
}
