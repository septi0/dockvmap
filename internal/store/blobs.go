package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) GetCachedBlob(ctx context.Context, digest string) (*model.CachedBlob, error) {
	var blob model.CachedBlob

	err := s.db.QueryRowContext(ctx, `
		SELECT digest, size, content_type, created_at, accessed_at
		FROM cached_blobs WHERE digest = ?
	`, digest).Scan(
		&blob.Digest,
		&blob.Size,
		&blob.ContentType,
		&blob.CreatedAt,
		&blob.AccessedAt,
	)

	if err == nil {
		return &blob, nil
	}

	if err == sql.ErrNoRows {
		return nil, nil
	}

	return nil, fmt.Errorf("getting cached blob %q: %w", digest, err)
}

func (s *Store) SaveCachedBlob(ctx context.Context, blob model.CachedBlob) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cached_blobs (digest, size, content_type, created_at, accessed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO UPDATE SET
			size = excluded.size,
			content_type = excluded.content_type,
			created_at = excluded.created_at,
			accessed_at = excluded.accessed_at
	`, blob.Digest, blob.Size, blob.ContentType, blob.CreatedAt, blob.AccessedAt)

	if err != nil {
		return fmt.Errorf("saving cached blob %q: %w", blob.Digest, err)
	}

	return nil
}

func (s *Store) TouchCachedBlob(ctx context.Context, digest string, accessedAt time.Time) error {
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE cached_blobs SET accessed_at = ? WHERE digest = ?",
		accessedAt,
		digest,
	)

	if err != nil {
		return fmt.Errorf("touching cached blob %q: %w", digest, err)
	}

	return nil
}

func (s *Store) DeleteCachedBlob(ctx context.Context, digest string) error {
	_, err := s.db.ExecContext(
		ctx,
		"DELETE FROM cached_blobs WHERE digest = ?",
		digest,
	)

	if err != nil {
		return fmt.Errorf("deleting cached blob %q: %w", digest, err)
	}

	return nil
}

func (s *Store) ListExpiredCachedBlobs(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT digest FROM cached_blobs WHERE accessed_at <= ?",
		before,
	)

	if err != nil {
		return nil, fmt.Errorf("listing expired cached blobs: %w", err)
	}

	defer rows.Close()

	var digests []string

	for rows.Next() {
		var digest string

		if err := rows.Scan(&digest); err != nil {
			return nil, fmt.Errorf("scanning expired cached blob: %w", err)
		}

		digests = append(digests, digest)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing expired cached blobs: %w", err)
	}

	return digests, nil
}

func (s *Store) ListCachedBlobDigestsWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT digest FROM cached_blobs WHERE digest >= ? AND digest < ?",
		prefix, prefixUpperBound(prefix),
	)

	if err != nil {
		return nil, fmt.Errorf("listing cached blob digests with prefix %q: %w", prefix, err)
	}

	defer rows.Close()

	var digests []string

	for rows.Next() {
		var digest string

		if err := rows.Scan(&digest); err != nil {
			return nil, fmt.Errorf("scanning cached blob digest: %w", err)
		}

		digests = append(digests, digest)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing cached blob digests with prefix %q: %w", prefix, err)
	}

	return digests, nil
}

func (s *Store) SumCachedBlobSize(ctx context.Context) (int64, error) {
	var total int64

	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(size), 0) FROM cached_blobs").Scan(&total); err != nil {
		return 0, fmt.Errorf("summing cached blob size: %w", err)
	}

	return total, nil
}

func (s *Store) ListCachedBlobsByAccessAsc(ctx context.Context, limit int) ([]model.CachedBlob, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT digest, size, accessed_at
		FROM cached_blobs
		ORDER BY accessed_at ASC, digest ASC
		LIMIT ?
	`, limit)

	if err != nil {
		return nil, fmt.Errorf("listing cached blobs by access time: %w", err)
	}

	defer rows.Close()

	var blobs []model.CachedBlob

	for rows.Next() {
		var blob model.CachedBlob

		if err := rows.Scan(&blob.Digest, &blob.Size, &blob.AccessedAt); err != nil {
			return nil, fmt.Errorf("scanning cached blob: %w", err)
		}

		blobs = append(blobs, blob)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing cached blobs by access time: %w", err)
	}

	return blobs, nil
}

func prefixUpperBound(prefix string) string {
	b := []byte(prefix)
	b[len(b)-1]++

	return string(b)
}

func (s *Store) DeleteCachedBlobIfExpired(ctx context.Context, digest string, before time.Time) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM cached_blobs WHERE digest = ? AND accessed_at <= ?",
		digest,
		before,
	)

	if err != nil {
		return false, fmt.Errorf("deleting expired cached blob %q: %w", digest, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted cached blob %q: %w", digest, err)
	}

	return deleted != 0, nil
}
