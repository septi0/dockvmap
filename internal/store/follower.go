package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) FollowerState(ctx context.Context) (model.FollowerState, error) {
	var state model.FollowerState
	var clusterID sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT cluster_id, applied_version, applied_tag, applied_at, initial_copy_done FROM follower_state WHERE id = 1
	`).Scan(&clusterID, &state.AppliedVersion, &state.AppliedTag, &state.AppliedAt, &state.InitialCopyDone)

	if err != nil {
		return state, fmt.Errorf("reading the follower state: %w", err)
	}

	state.ClusterID = clusterID.String

	return state, nil
}

// FollowerHasLocalData reports data a first sync would replace; the built-in registries without a login and the internal registry don't count
func (s *Store) FollowerHasLocalData(ctx context.Context) (bool, error) {
	var has bool

	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM images) OR EXISTS (SELECT 1 FROM image_groups) OR EXISTS (SELECT 1 FROM proxy_tokens)
			OR EXISTS (SELECT 1 FROM registries WHERE internal = 0 AND (registry NOT IN ('docker.io', 'ghcr.io') OR COALESCE(username, '') <> ''))
	`).Scan(&has)

	if err != nil {
		return false, fmt.Errorf("checking for local data: %w", err)
	}

	return has, nil
}

func (s *Store) ApplyReplicationSnapshot(ctx context.Context, snapshot *model.ReplicationSnapshot, internal *model.LocalInternalRegistry) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return 0, fmt.Errorf("starting the snapshot apply: %w", err)
	}

	defer tx.Rollback()

	var appliedCluster sql.NullString

	if err := tx.QueryRowContext(ctx, `SELECT cluster_id FROM follower_state WHERE id = 1`).Scan(&appliedCluster); err != nil {
		return 0, fmt.Errorf("reading the follower state: %w", err)
	}

	apply := snapshotApply{store: s, tx: tx, ctx: ctx}

	// rows from another cluster, or a node's own from before it followed, share ids but not meaning with the primary's
	if appliedCluster.String != snapshot.State.ClusterID {
		apply.exec(`DELETE FROM images`)
		apply.exec(`DELETE FROM image_groups`)
		apply.exec(`DELETE FROM proxy_tokens`)
		apply.exec(`DELETE FROM registries WHERE internal = 0`)
	}

	apply.deleteMissing("images", ids(snapshot.Images, func(i model.ReplicatedImage) int64 { return i.ID }))
	apply.deleteMissing("image_groups", ids(snapshot.Groups, func(g model.ReplicatedGroup) int64 { return g.ID }))
	apply.deleteMissing("proxy_tokens", ids(snapshot.ProxyTokens, func(t model.ReplicatedProxyToken) int64 { return t.ID }))
	apply.deleteMissing("registries", ids(snapshot.Registries, func(r model.ReplicatedRegistry) int64 { return r.ID }))

	for _, registry := range snapshot.Registries {
		if registry.Internal {
			if internal == nil {
				return 0, errors.New("the primary has an internal registry and this follower has none")
			}

			// the follower's own internal registry row takes the primary's id, with this node's address
			apply.exec(`DELETE FROM registries WHERE internal = 1 AND id <> ?`, registry.ID)
			registry = model.ReplicatedRegistry{ID: registry.ID, Registry: internal.Address, Options: internal.Options, Internal: true}
		}

		apply.upsertRegistry(registry)
	}

	for _, group := range snapshot.Groups {
		apply.exec(`
			INSERT INTO image_groups (id, name) VALUES (?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, updated_at = CURRENT_TIMESTAMP
			WHERE image_groups.name IS NOT excluded.name
		`, group.ID, group.Name)
	}

	for _, img := range snapshot.Images {
		apply.exec(`
			INSERT INTO images (id, name, registry_id, repository, tag, pinned, group_id) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = excluded.name, registry_id = excluded.registry_id, repository = excluded.repository,
				tag = excluded.tag, pinned = excluded.pinned, group_id = excluded.group_id
			WHERE images.name IS NOT excluded.name OR images.registry_id IS NOT excluded.registry_id OR images.repository IS NOT excluded.repository
				OR images.tag IS NOT excluded.tag OR images.pinned IS NOT excluded.pinned OR images.group_id IS NOT excluded.group_id
		`, img.ID, img.Name, img.RegistryID, img.Repository, img.Tag, img.Pinned, img.GroupID)
	}

	for _, token := range snapshot.ProxyTokens {
		apply.exec(`
			INSERT INTO proxy_tokens (id, label, token_hash, can_push, created_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET label = excluded.label, token_hash = excluded.token_hash, can_push = excluded.can_push
			WHERE proxy_tokens.label IS NOT excluded.label OR proxy_tokens.token_hash IS NOT excluded.token_hash OR proxy_tokens.can_push IS NOT excluded.can_push
		`, token.ID, token.Label, token.TokenHash, token.CanPush, token.CreatedAt.UTC())
	}

	if apply.err != nil {
		return 0, apply.err
	}

	state := snapshot.State

	if _, err := tx.ExecContext(ctx, `
		UPDATE follower_state SET cluster_id = ?, applied_version = ?, applied_tag = ?, applied_at = ?
		WHERE id = 1 AND (cluster_id IS NOT ? OR applied_version IS NOT ? OR applied_tag IS NOT ? OR ?)
	`, state.ClusterID, state.Version, state.Tag, time.Now().UTC(), state.ClusterID, state.Version, state.Tag, apply.changed > 0); err != nil {
		return 0, fmt.Errorf("saving the applied version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing the snapshot apply: %w", err)
	}

	return apply.changed, nil
}

type snapshotApply struct {
	store   *Store
	tx      *sql.Tx
	ctx     context.Context
	changed int64
	err     error
}

func (a *snapshotApply) exec(query string, args ...any) {
	if a.err != nil {
		return
	}

	result, err := a.tx.ExecContext(a.ctx, query, args...)

	if err != nil {
		a.err = fmt.Errorf("applying the snapshot: %w", err)
		return
	}

	n, _ := result.RowsAffected()
	a.changed += n
}

func (a *snapshotApply) deleteMissing(table string, keep []int64) {
	keepJSON, _ := json.Marshal(keep)

	a.exec(`DELETE FROM `+table+` WHERE id NOT IN (SELECT value FROM json_each(?))`, string(keepJSON))
}

func (a *snapshotApply) upsertRegistry(want model.ReplicatedRegistry) {
	if a.err != nil {
		return
	}

	optionsJSON, err := json.Marshal(want.Options)

	if err != nil {
		a.err = fmt.Errorf("marshalling registry options: %w", err)
		return
	}

	var host, options string
	var username sql.NullString
	var internal bool
	var nonce, ciphertext []byte

	err = a.tx.QueryRowContext(a.ctx, `
		SELECT registry, username, credential_nonce, credential_ciphertext, COALESCE(options, ''), internal FROM registries WHERE id = ?
	`, want.ID).Scan(&host, &username, &nonce, &ciphertext, &options, &internal)

	exists := err == nil

	if err != nil && !isNoRows(err) {
		a.err = fmt.Errorf("reading registry %d: %w", want.ID, err)
		return
	}

	if exists {
		credential, err := a.store.decryptRegistryCredential(want.ID, nonce, ciphertext)

		if err != nil {
			a.err = fmt.Errorf("decrypting the login of %s: %w", host, err)
			return
		}

		if host == want.Registry && username.String == want.Username && credential == want.Credential && options == string(optionsJSON) && internal == want.Internal {
			return
		}
	}

	newNonce, newCiphertext, err := a.store.encryptRegistryCredential(want.ID, want.Credential)

	if err != nil {
		a.err = fmt.Errorf("encrypting the login of %s: %w", want.Registry, err)
		return
	}

	now := time.Now().UTC()

	a.exec(`
		INSERT INTO registries (id, registry, username, credential_nonce, credential_ciphertext, options, internal, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET registry = excluded.registry, username = excluded.username,
			credential_nonce = excluded.credential_nonce, credential_ciphertext = excluded.credential_ciphertext,
			options = excluded.options, internal = excluded.internal, updated_at = excluded.updated_at
	`, want.ID, want.Registry, want.Username, newNonce, newCiphertext, string(optionsJSON), want.Internal, now, now)
}

func ids[T any](items []T, id func(T) int64) []int64 {
	out := make([]int64, 0, len(items))

	for _, item := range items {
		out = append(out, id(item))
	}

	return out
}

func (s *Store) ReplicatedImages(ctx context.Context) ([]model.ReplicatedImage, error) {
	var images []model.ReplicatedImage

	if err := collectRows(ctx, s.db, replicatedImagesQuery, &images, scanReplicatedImage); err != nil {
		return nil, fmt.Errorf("reading images: %w", err)
	}

	return images, nil
}

func (s *Store) FollowerRepositories(ctx context.Context) (map[string]int64, error) {
	var rows []model.ReplicatedRepository

	err := collectRows(ctx, s.db, `SELECT repository, counter FROM follower_repositories`, &rows, func(scan rowScanner, r *model.ReplicatedRepository) error {
		return scan.Scan(&r.Repository, &r.Counter)
	})

	if err != nil {
		return nil, fmt.Errorf("reading copied repositories: %w", err)
	}

	counters := make(map[string]int64, len(rows))

	for _, row := range rows {
		counters[row.Repository] = row.Counter
	}

	return counters, nil
}

func (s *Store) SetFollowerRepository(ctx context.Context, repository string, counter int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO follower_repositories (repository, counter) VALUES (?, ?)
		ON CONFLICT (repository) DO UPDATE SET counter = excluded.counter
	`, repository, counter)

	if err != nil {
		return fmt.Errorf("saving the copied counter of %s: %w", repository, err)
	}

	return nil
}

func (s *Store) MarkInitialCopyDone(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE follower_state SET initial_copy_done = 1 WHERE id = 1 AND initial_copy_done = 0`); err != nil {
		return fmt.Errorf("saving the initial copy: %w", err)
	}

	return nil
}

// RequestResync also forgets what was copied, so the registry is compared in full against the primary it resyncs from
func (s *Store) RequestResync(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("starting resync transaction: %w", err)
	}

	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE follower_state SET resync_requested = 1, initial_copy_done = 0 WHERE id = 1`); err != nil {
		return fmt.Errorf("requesting a resync: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM follower_repositories`); err != nil {
		return fmt.Errorf("forgetting copied repositories: %w", err)
	}

	return tx.Commit()
}

func (s *Store) TakeResyncRequest(ctx context.Context) (bool, error) {
	var requested bool

	if err := s.db.QueryRowContext(ctx, `SELECT resync_requested FROM follower_state WHERE id = 1`).Scan(&requested); err != nil || !requested {
		return false, err
	}

	result, err := s.db.ExecContext(ctx, `UPDATE follower_state SET resync_requested = 0 WHERE id = 1 AND resync_requested = 1`)

	if err != nil {
		return false, fmt.Errorf("reading the resync request: %w", err)
	}

	n, err := result.RowsAffected()

	return n > 0, err
}
