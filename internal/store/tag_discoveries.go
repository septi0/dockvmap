package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) GetTagDiscoveryByID(ctx context.Context, id int64) (*model.TagDiscovery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, registry_id, repository, status, result_json, tag_count, raw_tag_count, error, started_at, completed_at
		FROM tag_discoveries WHERE id = ?
	`, id)

	return scanTagDiscovery(row)
}

func (s *Store) GetTagDiscoveryByRegistryRepo(ctx context.Context, registryID int64, repository string) (*model.TagDiscovery, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, registry_id, repository, status, result_json, tag_count, raw_tag_count, error, started_at, completed_at
		FROM tag_discoveries WHERE registry_id = ? AND repository = ?
	`, registryID, repository)

	return scanTagDiscovery(row)
}

func scanTagDiscovery(row *sql.Row) (*model.TagDiscovery, error) {
	var d model.TagDiscovery
	var status string
	var resultJSON sql.NullString
	var tagCount sql.NullInt64
	var rawTagCount sql.NullInt64
	var errText sql.NullString
	var completedAt sql.NullTime

	err := row.Scan(&d.ID, &d.RegistryID, &d.Repository, &status, &resultJSON, &tagCount, &rawTagCount, &errText, &d.StartedAt, &completedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting tag discovery: %w", err)
	}

	d.Status = model.TagDiscoveryStatus(status)

	if resultJSON.Valid && resultJSON.String != "" {
		if err := json.Unmarshal([]byte(resultJSON.String), &d.TagGroups); err != nil {
			return nil, fmt.Errorf("decoding tag discovery result: %w", err)
		}
	}

	if tagCount.Valid {
		d.TagCount = int(tagCount.Int64)
	}

	if rawTagCount.Valid {
		d.RawTagCount = int(rawTagCount.Int64)
	}

	if errText.Valid {
		d.Error = errText.String
	}

	if completedAt.Valid {
		completed := completedAt.Time
		d.CompletedAt = &completed
	}

	return &d, nil
}

func (s *Store) StartOrGetTagDiscovery(ctx context.Context, registryID int64, repository string) (*model.TagDiscovery, bool, error) {
	now := time.Now().UTC()

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO tag_discoveries (registry_id, repository, status, started_at)
		VALUES (?, ?, ?, ?)
	`, registryID, repository, model.TagDiscoveryRunning, now)

	if err == nil {
		id, err := result.LastInsertId()

		if err != nil {
			return nil, false, fmt.Errorf("getting created tag discovery id: %w", err)
		}

		discovery, err := s.GetTagDiscoveryByID(ctx, id)

		if err != nil {
			return nil, false, err
		}

		return discovery, true, nil
	}

	if !isUniqueConstraintErr(err) {
		return nil, false, fmt.Errorf("creating tag discovery: %w", err)
	}

	reclaimed, err := s.db.ExecContext(ctx, `
		UPDATE tag_discoveries
		SET status = ?, error = NULL, started_at = ?, completed_at = NULL
		WHERE registry_id = ? AND repository = ? AND status = ?
	`, model.TagDiscoveryRunning, now, registryID, repository, model.TagDiscoveryFailed)

	if err != nil {
		return nil, false, fmt.Errorf("reclaiming tag discovery: %w", err)
	}

	affected, err := reclaimed.RowsAffected()

	if err != nil {
		return nil, false, fmt.Errorf("checking reclaimed tag discovery: %w", err)
	}

	discovery, err := s.GetTagDiscoveryByRegistryRepo(ctx, registryID, repository)

	if err != nil {
		return nil, false, err
	}

	if discovery == nil {
		return nil, false, fmt.Errorf("tag discovery for registry %d repository %q disappeared unexpectedly", registryID, repository)
	}

	return discovery, affected > 0, nil
}

func (s *Store) CompleteTagDiscovery(ctx context.Context, id int64, groups []model.TagDiscoveryGroup, tagCount int, rawTagCount int) error {
	if groups == nil {
		groups = []model.TagDiscoveryGroup{}
	}

	resultJSON, err := json.Marshal(groups)

	if err != nil {
		return fmt.Errorf("marshalling tag discovery result: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE tag_discoveries
		SET status = ?, result_json = ?, tag_count = ?, raw_tag_count = ?, error = NULL, completed_at = ?
		WHERE id = ?
	`, model.TagDiscoveryCompleted, string(resultJSON), tagCount, rawTagCount, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("completing tag discovery: %w", err)
	}

	return nil
}

func (s *Store) FailTagDiscovery(ctx context.Context, id int64, errMessage string) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tag_discoveries
		SET status = ?, error = ?, completed_at = ?
		WHERE id = ?
	`, model.TagDiscoveryFailed, errMessage, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("failing tag discovery: %w", err)
	}

	return nil
}

func (s *Store) RecordTagDiscoveryRefreshFailure(ctx context.Context, id int64, errMessage string) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE tag_discoveries
		SET error = ?, completed_at = ?
		WHERE id = ? AND status = ?
	`, errMessage, time.Now().UTC(), id, model.TagDiscoveryCompleted); err != nil {
		return fmt.Errorf("recording tag discovery refresh failure: %w", err)
	}

	return nil
}

func (s *Store) MarkStaleRunningTagDiscoveriesAsFailed(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE tag_discoveries
		SET status = ?, error = ?, completed_at = ?
		WHERE status = ?
	`, model.TagDiscoveryFailed, "interrupted by restart", time.Now().UTC(), model.TagDiscoveryRunning)

	if err != nil {
		return 0, fmt.Errorf("marking stale tag discoveries as failed: %w", err)
	}

	return result.RowsAffected()
}

func (s *Store) DeleteOldTagDiscoveries(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM tag_discoveries
		WHERE status IN (?, ?) AND completed_at < ?
	`, model.TagDiscoveryCompleted, model.TagDiscoveryFailed, olderThan)

	if err != nil {
		return 0, fmt.Errorf("deleting old tag discoveries: %w", err)
	}

	return result.RowsAffected()
}
