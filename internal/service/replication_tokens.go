package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const (
	replicationTokenTouchEvery = time.Hour
	maxReplicationTokenLabel   = 100
)

var ErrInvalidReplicationToken = errors.New("invalid replication token")

type auditReplicationTokenData struct {
	Label string `json:"label"`
}

func (r *Replication) Authenticate(ctx context.Context, secret string) (*model.ReplicationToken, error) {
	if secret == "" {
		return nil, nil
	}

	token, err := r.store.ReplicationTokenByHash(ctx, hashToken(secret))

	if err != nil || token == nil {
		return nil, err
	}

	if token.LastUsedAt == nil || time.Since(*token.LastUsedAt) > replicationTokenTouchEvery {
		if err := r.store.TouchReplicationToken(ctx, token.ID, time.Now()); err != nil {
			slog.Warn("recording a replication token's use failed", "error", err)
		}
	}

	return token, nil
}

func (r *Replication) CreateToken(ctx context.Context, label string) (int64, string, error) {
	label = strings.TrimSpace(label)

	if label == "" || len(label) > maxReplicationTokenLabel {
		return 0, "", fmt.Errorf("%w: a label of 1 to %d characters is required", ErrInvalidReplicationToken, maxReplicationTokenLabel)
	}

	token, secret, err := replicationapi.NewToken(r.fingerprint)

	if err != nil {
		return 0, "", fmt.Errorf("generating replication token: %w", err)
	}

	id, err := r.store.CreateReplicationToken(ctx, label, hashToken(secret))

	if err != nil {
		return 0, "", err
	}

	recordAudit(ctx, r.audit, AuditTypeReplicationTokenCreated, auditReplicationTokenData{Label: label})

	return id, token, nil
}

func (r *Replication) ListTokens(ctx context.Context) ([]model.ReplicationToken, error) {
	return r.store.ListReplicationTokens(ctx)
}

func (r *Replication) DeleteToken(ctx context.Context, id int64) (bool, error) {
	token, err := r.store.GetReplicationTokenByID(ctx, id)

	if err != nil || token == nil {
		return false, err
	}

	deleted, err := r.store.DeleteReplicationToken(ctx, id)

	if err != nil || !deleted {
		return false, err
	}

	recordAudit(ctx, r.audit, AuditTypeReplicationTokenDeleted, auditReplicationTokenData{Label: token.Label})

	return true, nil
}
