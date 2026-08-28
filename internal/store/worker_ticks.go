package store

import (
	"context"
	"fmt"
	"time"
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

func (s *Store) RecordWorkerTick(ctx context.Context, job string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO worker_ticks (job, last_run_at)
		VALUES (?, ?)
		ON CONFLICT(job) DO UPDATE SET last_run_at = excluded.last_run_at
	`, job, at); err != nil {
		return fmt.Errorf("recording worker tick %q: %w", job, err)
	}

	return nil
}
