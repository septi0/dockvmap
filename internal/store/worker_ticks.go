package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) GetWorkerTick(ctx context.Context, job string) (time.Time, bool, error) {
	var lastRun time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT last_run_at
		FROM worker_ticks
		WHERE job = ?
	`, job).Scan(&lastRun)

	if isNoRows(err) {
		return time.Time{}, false, nil
	}

	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading worker tick %q: %w", job, err)
	}

	return lastRun, true, nil
}

func (s *Store) ListWorkerTicks(ctx context.Context) ([]model.WorkerTick, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT job, last_run_at, last_error, last_count
		FROM worker_ticks
	`)

	if err != nil {
		return nil, fmt.Errorf("listing worker ticks: %w", err)
	}

	defer rows.Close()

	ticks := make([]model.WorkerTick, 0)

	for rows.Next() {
		var (
			tick      model.WorkerTick
			lastError sql.NullString
			lastCount sql.NullInt64
		)

		if err := rows.Scan(&tick.Job, &tick.LastRunAt, &lastError, &lastCount); err != nil {
			return nil, fmt.Errorf("scanning worker tick row: %w", err)
		}

		tick.LastError = lastError.String

		if lastCount.Valid {
			tick.LastCount = &lastCount.Int64
		}

		ticks = append(ticks, tick)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading worker tick rows: %w", err)
	}

	return ticks, nil
}

func (s *Store) RecordWorkerRun(ctx context.Context, job string, at time.Time, count int64, errText string) error {
	var errArg any
	if errText != "" {
		errArg = errText
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO worker_ticks (job, last_run_at, last_count, last_error)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(job) DO UPDATE SET
			last_run_at = excluded.last_run_at,
			last_count = excluded.last_count,
			last_error = excluded.last_error
	`, job, at, count, errArg); err != nil {
		return fmt.Errorf("recording worker run %q: %w", job, err)
	}

	return nil
}
