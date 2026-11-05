package store

import (
	"context"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) RecordProxyUsage(ctx context.Context, images []model.ImageUsage, tokens []model.ProxyTokenUsage) error {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("starting proxy usage transaction: %w", err)
	}

	defer tx.Rollback()

	for _, usage := range images {
		_, err := tx.ExecContext(ctx, "UPDATE images SET last_pulled_at = ?, last_pulled_tag = ? WHERE id = ?", usage.PulledAt, usage.PulledTag, usage.ImageID)

		if err != nil {
			return fmt.Errorf("recording usage for image %d: %w", usage.ImageID, err)
		}
	}

	for _, usage := range tokens {
		if _, err := tx.ExecContext(ctx, "UPDATE proxy_tokens SET last_used_at = ? WHERE id = ?", usage.UsedAt, usage.TokenID); err != nil {
			return fmt.Errorf("recording usage for proxy token %d: %w", usage.TokenID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing proxy usage: %w", err)
	}

	return nil
}
