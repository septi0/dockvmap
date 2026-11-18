package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

// every table a snapshot carries; each must have the version-bump triggers from migration 037
var replicatedTables = []string{"registries", "image_groups", "images", "proxy_tokens", "replication_repositories"}

func (s *Store) ReplicationState(ctx context.Context) (model.ReplicationState, error) {
	return replicationState(ctx, s.db)
}

func replicationState(ctx context.Context, q DBTX) (model.ReplicationState, error) {
	var state model.ReplicationState
	var clusterID sql.NullString

	err := q.QueryRowContext(ctx, `SELECT cluster_id, version, tag FROM replication_state WHERE id = 1`).Scan(&clusterID, &state.Version, &state.Tag)

	if err != nil {
		return state, fmt.Errorf("reading the replication state: %w", err)
	}

	state.ClusterID = clusterID.String

	return state, nil
}

func (s *Store) EnsureClusterID(ctx context.Context, newID string) (string, error) {
	if _, err := s.db.ExecContext(ctx, `UPDATE replication_state SET cluster_id = ? WHERE id = 1 AND cluster_id IS NULL`, newID); err != nil {
		return "", fmt.Errorf("saving the cluster ID: %w", err)
	}

	state, err := s.ReplicationState(ctx)

	if err != nil {
		return "", err
	}

	return state.ClusterID, nil
}

func (s *Store) ClearClusterID(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE replication_state SET cluster_id = NULL WHERE id = 1`); err != nil {
		return fmt.Errorf("clearing the cluster ID: %w", err)
	}

	return nil
}

func (s *Store) RecordRepositoryChange(ctx context.Context, repository, reference string, deleted bool) error {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("starting repository change transaction: %w", err)
	}

	defer tx.Rollback()

	var counter int64

	err = tx.QueryRowContext(ctx, `
		INSERT INTO replication_repositories (repository, counter) VALUES (?, 1)
		ON CONFLICT (repository) DO UPDATE SET counter = counter + 1
		RETURNING counter
	`, repository).Scan(&counter)

	if err != nil {
		return fmt.Errorf("bumping the change counter of %s: %w", repository, err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO replication_changes (repository, reference, deleted, counter) VALUES (?, ?, ?, ?)
	`, repository, reference, deleted, counter); err != nil {
		return fmt.Errorf("recording the change of %s:%s: %w", repository, reference, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing repository change: %w", err)
	}

	return nil
}

func (s *Store) CleanupReplicationChanges(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM replication_changes WHERE changed_at < ?`, sqliteDatetime(olderThan))

	if err != nil {
		return 0, fmt.Errorf("removing old recorded changes: %w", err)
	}

	return result.RowsAffected()
}

// RepositoryChanges returns the changes after the given counter; complete is false when some were pruned or the counter is ahead of this primary's
func (s *Store) RepositoryChanges(ctx context.Context, repository string, after int64) ([]model.RepositoryChange, bool, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})

	if err != nil {
		return nil, false, fmt.Errorf("starting changes transaction: %w", err)
	}

	defer tx.Rollback()

	var current int64

	if err := tx.QueryRowContext(ctx, `SELECT counter FROM replication_repositories WHERE repository = ?`, repository).Scan(&current); err != nil && !isNoRows(err) {
		return nil, false, fmt.Errorf("reading the counter of %s: %w", repository, err)
	}

	var changes []model.RepositoryChange

	err = collectRows(ctx, tx, `SELECT reference, deleted, counter FROM replication_changes WHERE repository = ? AND counter > ? ORDER BY counter`, &changes, func(scan rowScanner, c *model.RepositoryChange) error {
		return scan.Scan(&c.Reference, &c.Deleted, &c.Counter)
	}, repository, after)

	if err != nil {
		return nil, false, fmt.Errorf("reading the changes of %s: %w", repository, err)
	}

	return changes, current >= after && int64(len(changes)) == current-after, nil
}

func (s *Store) ReplicationSnapshot(ctx context.Context) (*model.ReplicationSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})

	if err != nil {
		return nil, fmt.Errorf("starting snapshot transaction: %w", err)
	}

	defer tx.Rollback()

	snapshot := &model.ReplicationSnapshot{}

	if snapshot.State, err = replicationState(ctx, tx); err != nil {
		return nil, err
	}

	if snapshot.Registries, err = s.snapshotRegistries(ctx, tx); err != nil {
		return nil, err
	}

	err = collectRows(ctx, tx, `SELECT id, name FROM image_groups ORDER BY id`, &snapshot.Groups, func(scan rowScanner, g *model.ReplicatedGroup) error {
		return scan.Scan(&g.ID, &g.Name)
	})

	if err != nil {
		return nil, fmt.Errorf("reading groups for the snapshot: %w", err)
	}

	if err = collectRows(ctx, tx, replicatedImagesQuery, &snapshot.Images, scanReplicatedImage); err != nil {
		return nil, fmt.Errorf("reading images for the snapshot: %w", err)
	}

	err = collectRows(ctx, tx, `SELECT id, label, token_hash, can_push, created_at FROM proxy_tokens ORDER BY id`, &snapshot.ProxyTokens, func(scan rowScanner, t *model.ReplicatedProxyToken) error {
		return scan.Scan(&t.ID, &t.Label, &t.TokenHash, &t.CanPush, &t.CreatedAt)
	})

	if err != nil {
		return nil, fmt.Errorf("reading proxy tokens for the snapshot: %w", err)
	}

	err = collectRows(ctx, tx, `SELECT repository, counter FROM replication_repositories ORDER BY repository`, &snapshot.Repositories, func(scan rowScanner, r *model.ReplicatedRepository) error {
		return scan.Scan(&r.Repository, &r.Counter)
	})

	if err != nil {
		return nil, fmt.Errorf("reading repository counters for the snapshot: %w", err)
	}

	return snapshot, nil
}

const replicatedImagesQuery = `SELECT id, name, registry_id, repository, tag, pinned, group_id FROM images ORDER BY id`

func scanReplicatedImage(scan rowScanner, img *model.ReplicatedImage) error {
	return scan.Scan(&img.ID, &img.Name, &img.RegistryID, &img.Repository, &img.Tag, &img.Pinned, &img.GroupID)
}

func (s *Store) snapshotRegistries(ctx context.Context, tx *sql.Tx) ([]model.ReplicatedRegistry, error) {
	registries := make([]model.ReplicatedRegistry, 0)

	rows, err := tx.QueryContext(ctx, `
		SELECT id, registry, username, credential_nonce, credential_ciphertext, options, internal FROM registries ORDER BY id
	`)

	if err != nil {
		return nil, fmt.Errorf("reading registries for the snapshot: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var registry model.ReplicatedRegistry
		var username, options sql.NullString
		var nonce, ciphertext []byte

		if err := rows.Scan(&registry.ID, &registry.Registry, &username, &nonce, &ciphertext, &options, &registry.Internal); err != nil {
			return nil, fmt.Errorf("scanning registry for the snapshot: %w", err)
		}

		registry.Username = username.String
		registry.Options = parseRegistryOptions(options.String)

		if registry.Credential, err = s.decryptRegistryCredential(registry.ID, nonce, ciphertext); err != nil {
			return nil, fmt.Errorf("decrypting the login of %s for the snapshot: %w", registry.Registry, err)
		}

		registries = append(registries, registry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading registries for the snapshot: %w", err)
	}

	return registries, nil
}

func collectRows[T any](ctx context.Context, q DBTX, query string, out *[]T, scan func(rowScanner, *T) error, args ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)

	if err != nil {
		return err
	}

	defer rows.Close()

	items := make([]T, 0)

	for rows.Next() {
		var item T

		if err := scan(rows, &item); err != nil {
			return err
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	*out = items

	return nil
}

func (s *Store) CreateReplicationToken(ctx context.Context, label, tokenHash string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO replication_tokens (label, token_hash) VALUES (?, ?)`, label, tokenHash)

	if err != nil {
		return 0, fmt.Errorf("creating replication token: %w", err)
	}

	return result.LastInsertId()
}

func (s *Store) ListReplicationTokens(ctx context.Context) ([]model.ReplicationToken, error) {
	var tokens []model.ReplicationToken

	err := collectRows(ctx, s.db, `SELECT id, label, created_at, last_used_at FROM replication_tokens ORDER BY created_at DESC`, &tokens, func(scan rowScanner, t *model.ReplicationToken) error {
		return scan.Scan(&t.ID, &t.Label, &t.CreatedAt, &t.LastUsedAt)
	})

	if err != nil {
		return nil, fmt.Errorf("listing replication tokens: %w", err)
	}

	return tokens, nil
}

func (s *Store) ReplicationTokenByHash(ctx context.Context, tokenHash string) (*model.ReplicationToken, error) {
	var token model.ReplicationToken

	err := s.db.QueryRowContext(ctx, `
		SELECT id, label, created_at, last_used_at FROM replication_tokens WHERE token_hash = ?
	`, tokenHash).Scan(&token.ID, &token.Label, &token.CreatedAt, &token.LastUsedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("checking replication token: %w", err)
	}

	return &token, nil
}

func (s *Store) GetReplicationTokenByID(ctx context.Context, id int64) (*model.ReplicationToken, error) {
	var token model.ReplicationToken

	err := s.db.QueryRowContext(ctx, `
		SELECT id, label, created_at, last_used_at FROM replication_tokens WHERE id = ?
	`, id).Scan(&token.ID, &token.Label, &token.CreatedAt, &token.LastUsedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting replication token: %w", err)
	}

	return &token, nil
}

func (s *Store) TouchReplicationToken(ctx context.Context, id int64, usedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE replication_tokens SET last_used_at = ? WHERE id = ?`, usedAt.UTC(), id); err != nil {
		return fmt.Errorf("recording replication token use: %w", err)
	}

	return nil
}

func (s *Store) DeleteReplicationToken(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM replication_tokens WHERE id = ?`, id)

	if err != nil {
		return false, fmt.Errorf("deleting replication token %d: %w", id, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted replication token %d: %w", id, err)
	}

	return deleted != 0, nil
}

func (s *Store) SaveReplicationFollower(ctx context.Context, f model.ReplicationFollower) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO replication_followers (follower_id, address, token_id, version, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (follower_id) DO UPDATE SET address = excluded.address, token_id = excluded.token_id, version = excluded.version, last_seen_at = excluded.last_seen_at
	`, f.FollowerID, f.Address, f.TokenID, f.Version, f.LastSeenAt.UTC(), f.LastSeenAt.UTC())

	if err != nil {
		return fmt.Errorf("saving follower %s: %w", f.Address, err)
	}

	return nil
}

const replicationFollowerSelect = `
	SELECT f.id, f.follower_id, f.name, COALESCE(f.token_id, 0), COALESCE(t.label, ''), f.address, f.version, f.first_seen_at, f.last_seen_at
	FROM replication_followers f LEFT JOIN replication_tokens t ON t.id = f.token_id`

func scanReplicationFollower(scan rowScanner, f *model.ReplicationFollower) error {
	return scan.Scan(&f.ID, &f.FollowerID, &f.Name, &f.TokenID, &f.TokenLabel, &f.Address, &f.Version, &f.FirstSeenAt, &f.LastSeenAt)
}

func (s *Store) ListReplicationFollowers(ctx context.Context) ([]model.ReplicationFollower, error) {
	var followers []model.ReplicationFollower

	if err := collectRows(ctx, s.db, replicationFollowerSelect+` ORDER BY COALESCE(NULLIF(f.name, ''), f.address), f.address`, &followers, scanReplicationFollower); err != nil {
		return nil, fmt.Errorf("listing followers: %w", err)
	}

	return followers, nil
}

func (s *Store) GetReplicationFollower(ctx context.Context, id int64) (*model.ReplicationFollower, error) {
	var follower model.ReplicationFollower

	err := scanReplicationFollower(s.db.QueryRowContext(ctx, replicationFollowerSelect+` WHERE f.id = ?`, id), &follower)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting follower %d: %w", id, err)
	}

	return &follower, nil
}

func (s *Store) RenameReplicationFollower(ctx context.Context, id int64, name string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE replication_followers SET name = ? WHERE id = ?`, name, id); err != nil {
		return fmt.Errorf("renaming follower %d: %w", id, err)
	}

	return nil
}

func (s *Store) DeleteReplicationFollower(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM replication_followers WHERE id = ?`, id)

	if err != nil {
		return false, fmt.Errorf("removing follower %d: %w", id, err)
	}

	deleted, err := result.RowsAffected()

	return deleted > 0, err
}
