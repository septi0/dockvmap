package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const (
	FollowerConnected = "connected"
	FollowerWaiting   = "waiting"
	FollowerMissing   = "missing"

	replicationFollowerSaveEvery = time.Minute
	maxFollowerName              = 100
)

var followerIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

var ErrInvalidFollowerName = errors.New("invalid follower name")

// Token is the label of the token the follower last used, empty once that token is deleted
type SeenFollower struct {
	ID             int64
	Name           string
	Address        string
	Token          string
	Version        string
	AppliedVersion int64
	Status         string
	Connection     string
	FirstSeen      time.Time
	LastSeen       time.Time
	Health         *replicationapi.FollowerHealth
}

// HasProblem is a follower that is gone, stopped syncing or failing copies; one still catching up is not
func (f SeenFollower) HasProblem() bool {
	switch {
	case f.Connection == FollowerMissing:
		return true
	case f.Connection != FollowerConnected:
		return false
	}

	return (f.Status != "" && f.Status != FollowerSyncRunning) || (f.Health != nil && f.Health.FailedCopies > 0)
}

// liveFollower is what the last request said; the saved row keeps only who it is and when it was seen
type liveFollower struct {
	row            model.ReplicationFollower
	appliedVersion int64
	status         string
	health         *replicationapi.FollowerHealth
	savedAt        time.Time
}

type auditReplicationFollowerData struct {
	Name    string `json:"name,omitempty"`
	OldName string `json:"oldName,omitempty"`
	NewName string `json:"newName,omitempty"`
	Address string `json:"address"`
}

// followerKey is the ID a follower sends, or its address when it's too old to send one
func followerKey(address string, health *replicationapi.FollowerHealth) string {
	if health != nil && followerIDPattern.MatchString(health.ID) {
		return health.ID
	}

	return "address-" + address
}

func (r *Replication) SeeFollower(ctx context.Context, token *model.ReplicationToken, address, version, status string, applied int64, health *replicationapi.FollowerHealth) {
	now := time.Now().UTC()
	row := model.ReplicationFollower{
		FollowerID: followerKey(address, health),
		Address:    address,
		TokenID:    token.ID,
		TokenLabel: token.Label,
		Version:    version,
		LastSeenAt: now,
	}

	r.mu.Lock()
	live, known := r.followers[row.FollowerID]

	if !known {
		live = &liveFollower{}
		r.followers[row.FollowerID] = live
	}

	old := live.row
	save := !known || old.Address != row.Address || old.TokenID != row.TokenID || old.Version != row.Version ||
		now.Sub(live.savedAt) >= replicationFollowerSaveEvery
	live.row, live.appliedVersion, live.status, live.health = row, applied, status, health

	if save {
		live.savedAt = now
	}

	r.mu.Unlock()

	if !save {
		return
	}

	if err := r.store.SaveReplicationFollower(ctx, row); err != nil {
		slog.Warn("saving a follower's last contact failed", "address", address, "error", err)

		r.mu.Lock()
		live.savedAt = time.Time{}
		r.mu.Unlock()
	}
}

// Followers lists every follower ever seen; one that hasn't contacted this primary since it started is waiting for 2 minutes, then missing
func (r *Replication) Followers(ctx context.Context) ([]SeenFollower, error) {
	rows, err := r.store.ListReplicationFollowers(ctx)

	if err != nil {
		return nil, err
	}

	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	followers := make([]SeenFollower, 0, len(rows))

	for _, row := range rows {
		live, ok := r.followers[row.FollowerID]

		if ok {
			row.Address, row.Version, row.LastSeenAt = live.row.Address, live.row.Version, live.row.LastSeenAt
		}

		f := SeenFollower{
			ID:        row.ID,
			Name:      row.Name,
			Address:   row.Address,
			Token:     row.TokenLabel,
			Version:   row.Version,
			FirstSeen: row.FirstSeenAt,
			LastSeen:  row.LastSeenAt,
		}

		switch {
		case ok && now.Sub(f.LastSeen) <= replicationFollowerSeenTTL:
			f.Connection = FollowerConnected
			f.AppliedVersion, f.Status, f.Health = live.appliedVersion, live.status, live.health
		case now.Sub(r.startedAt) <= replicationFollowerSeenTTL:
			f.Connection = FollowerWaiting
		default:
			f.Connection = FollowerMissing
		}

		followers = append(followers, f)
	}

	return followers, nil
}

// RenameFollower sets the name a follower is shown by; an empty name shows its address again
func (r *Replication) RenameFollower(ctx context.Context, id int64, name string) (bool, error) {
	name = strings.TrimSpace(name)

	if len(name) > maxFollowerName {
		return false, fmt.Errorf("%w: use at most %d characters", ErrInvalidFollowerName, maxFollowerName)
	}

	follower, err := r.store.GetReplicationFollower(ctx, id)

	if err != nil || follower == nil {
		return false, err
	}

	if follower.Name == name {
		return true, nil
	}

	if err := r.store.RenameReplicationFollower(ctx, id, name); err != nil {
		return false, err
	}

	recordAudit(ctx, r.audit, AuditTypeReplicationFollowerRenamed, auditReplicationFollowerData{OldName: follower.Name, NewName: name, Address: follower.Address})

	return true, nil
}

// RemoveFollower forgets a retired follower; one still running shows up again at its next request
func (r *Replication) RemoveFollower(ctx context.Context, id int64) (bool, error) {
	follower, err := r.store.GetReplicationFollower(ctx, id)

	if err != nil || follower == nil {
		return false, err
	}

	removed, err := r.store.DeleteReplicationFollower(ctx, id)

	if err != nil || !removed {
		return false, err
	}

	r.mu.Lock()
	delete(r.followers, follower.FollowerID)
	r.mu.Unlock()

	recordAudit(ctx, r.audit, AuditTypeReplicationFollowerRemoved, auditReplicationFollowerData{Name: follower.Name, Address: follower.Address})

	return true, nil
}
