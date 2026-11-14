package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

// RecordProxyUsage keeps the newer time per image and token, so a follower's late or repeated report never moves one back
func (s *Store) RecordProxyUsage(ctx context.Context, images []model.ImageUsage, tokens []model.ProxyTokenUsage) error {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("starting proxy usage transaction: %w", err)
	}

	defer tx.Rollback()

	for _, usage := range images {
		newer, err := isNewer(ctx, tx, `SELECT last_pulled_at FROM images WHERE id = ?`, usage.ImageID, usage.PulledAt)

		if err != nil {
			return fmt.Errorf("reading usage of image %d: %w", usage.ImageID, err)
		}

		if !newer {
			continue
		}

		if _, err := tx.ExecContext(ctx, "UPDATE images SET last_pulled_at = ?, last_pulled_tag = ? WHERE id = ?", usage.PulledAt, usage.PulledTag, usage.ImageID); err != nil {
			return fmt.Errorf("recording usage for image %d: %w", usage.ImageID, err)
		}
	}

	for _, usage := range tokens {
		newer, err := isNewer(ctx, tx, `SELECT last_used_at FROM proxy_tokens WHERE id = ?`, usage.TokenID, usage.UsedAt)

		if err != nil {
			return fmt.Errorf("reading usage of proxy token %d: %w", usage.TokenID, err)
		}

		if !newer {
			continue
		}

		if _, err := tx.ExecContext(ctx, "UPDATE proxy_tokens SET last_used_at = ? WHERE id = ?", usage.UsedAt, usage.TokenID); err != nil {
			return fmt.Errorf("recording usage for proxy token %d: %w", usage.TokenID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing proxy usage: %w", err)
	}

	return nil
}

// compared in Go: stored times are RFC 3339 text with a varying fraction, which doesn't sort as text
func isNewer(ctx context.Context, tx *sql.Tx, query string, id int64, at time.Time) (bool, error) {
	var current sql.NullTime

	err := tx.QueryRowContext(ctx, query, id).Scan(&current)

	if isNoRows(err) {
		return false, nil
	}

	return err == nil && (!current.Valid || at.After(current.Time)), err
}

// ProxyUsage returns the last pull of every image and the last use of every proxy token that has one
func (s *Store) ProxyUsage(ctx context.Context) ([]model.ImageUsage, []model.ProxyTokenUsage, error) {
	var images []model.ImageUsage
	var tokens []model.ProxyTokenUsage

	err := collectRows(ctx, s.db, `SELECT id, last_pulled_at, COALESCE(last_pulled_tag, '') FROM images WHERE last_pulled_at IS NOT NULL`, &images, func(scan rowScanner, u *model.ImageUsage) error {
		return scan.Scan(&u.ImageID, &u.PulledAt, &u.PulledTag)
	})

	if err != nil {
		return nil, nil, fmt.Errorf("reading image usage: %w", err)
	}

	err = collectRows(ctx, s.db, `SELECT id, last_used_at FROM proxy_tokens WHERE last_used_at IS NOT NULL`, &tokens, func(scan rowScanner, u *model.ProxyTokenUsage) error {
		return scan.Scan(&u.TokenID, &u.UsedAt)
	})

	if err != nil {
		return nil, nil, fmt.Errorf("reading proxy token usage: %w", err)
	}

	return images, tokens, nil
}
