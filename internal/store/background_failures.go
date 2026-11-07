package store

import (
	"context"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) InsertBackgroundFailure(ctx context.Context, source, detail, errText string) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO background_failures (source, detail, error)
		VALUES (?, ?, ?)
	`, source, detail, errText); err != nil {
		return fmt.Errorf("recording background failure: %w", err)
	}

	return nil
}

func backgroundFailureListWhere(filters model.BackgroundFailureListFilters) (string, []any) {
	b := &whereBuilder{}

	if filters.Source != "" {
		b.add("source = ?", filters.Source)
	}

	b.dateRange("occurred_at", filters.Since, filters.Until)

	return b.clause(), b.args
}

func (s *Store) CountBackgroundFailures(ctx context.Context, filters model.BackgroundFailureListFilters) (int64, error) {
	where, args := backgroundFailureListWhere(filters)

	return s.countWhere(ctx, "background_failures", where, args)
}

func (s *Store) ListBackgroundFailures(ctx context.Context, filters model.BackgroundFailureListFilters) ([]model.BackgroundFailure, error) {
	where, args := backgroundFailureListWhere(filters)

	query := fmt.Sprintf(`
		SELECT id, source, detail, error, occurred_at
		FROM background_failures
		%s
		ORDER BY occurred_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, where)

	rows, err := s.db.QueryContext(ctx, query, append(args, filters.Limit, filters.Offset)...)

	if err != nil {
		return nil, fmt.Errorf("listing background failures: %w", err)
	}

	defer rows.Close()

	failures := make([]model.BackgroundFailure, 0)

	for rows.Next() {
		f := model.BackgroundFailure{Occurrences: 1}

		if err := rows.Scan(&f.ID, &f.Source, &f.Detail, &f.Error, &f.OccurredAt); err != nil {
			return nil, fmt.Errorf("scanning background failure row: %w", err)
		}

		failures = append(failures, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading background failure rows: %w", err)
	}

	return failures, nil
}

func (s *Store) ListLatestDistinctBackgroundFailures(ctx context.Context, limit int) ([]model.BackgroundFailure, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.source, f.detail, f.error, f.occurred_at, latest.occurrences
		FROM background_failures f
		JOIN (
			SELECT MAX(id) AS id, COUNT(*) AS occurrences
			FROM background_failures
			GROUP BY source, detail, error
		) latest ON latest.id = f.id
		-- a tag check failure counts only while that image's last check still fails
		WHERE f.source != 'refresh' OR EXISTS (
			SELECT 1 FROM images i WHERE i.name = f.detail AND i.last_check_error IS NOT NULL
		)
		ORDER BY f.occurred_at DESC, f.id DESC
		LIMIT ?
	`, limit)

	if err != nil {
		return nil, fmt.Errorf("listing latest background failures: %w", err)
	}

	defer rows.Close()

	failures := make([]model.BackgroundFailure, 0)

	for rows.Next() {
		var f model.BackgroundFailure

		if err := rows.Scan(&f.ID, &f.Source, &f.Detail, &f.Error, &f.OccurredAt, &f.Occurrences); err != nil {
			return nil, fmt.Errorf("scanning background failure row: %w", err)
		}

		failures = append(failures, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading background failure rows: %w", err)
	}

	return failures, nil
}

func (s *Store) DeleteBackgroundFailuresBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM background_failures
		WHERE occurred_at < ?
	`, sqliteDatetime(cutoff))

	if err != nil {
		return 0, fmt.Errorf("deleting old background failures: %w", err)
	}

	return result.RowsAffected()
}
