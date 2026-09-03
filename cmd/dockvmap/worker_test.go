package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

func TestFirstRunDelayFor(t *testing.T) {
	const (
		interval = time.Hour
		offset   = 15 * time.Second
	)

	tests := []struct {
		name         string
		sinceLastRun time.Duration
		hasLastRun   bool
		want         time.Duration
	}{
		{"no prior run starts after the stagger offset", 0, false, offset},
		{"recent run waits the remaining interval plus offset", 20 * time.Minute, true, 40*time.Minute + offset},
		{"overdue run fires after just the offset", 3 * time.Hour, true, offset},
		{"exactly due run fires after just the offset", interval, true, offset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstRunDelayFor(interval, tt.sinceLastRun, offset, tt.hasLastRun); got != tt.want {
				t.Fatalf("firstRunDelayFor(%v, %v, %v, %v) = %v, want %v", interval, tt.sinceLastRun, offset, tt.hasLastRun, got, tt.want)
			}
		})
	}
}

func TestRescheduleDelay(t *testing.T) {
	const interval = time.Hour

	tests := []struct {
		name    string
		elapsed time.Duration
		want    time.Duration
	}{
		{"quick run reschedules for the remainder", time.Minute, 59 * time.Minute},
		{"run longer than the interval reschedules immediately", 2 * time.Hour, 0},
		{"run exactly the interval reschedules immediately", interval, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rescheduleDelay(interval, tt.elapsed); got != tt.want {
				t.Fatalf("rescheduleDelay(%v, %v) = %v, want %v", interval, tt.elapsed, got, tt.want)
			}
		})
	}
}

type recordedOutcome struct {
	job   string
	count int64
	err   string
}

type recordingScheduleStore struct {
	outcomes []recordedOutcome
}

func (s *recordingScheduleStore) GetWorkerTick(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (s *recordingScheduleStore) ListWorkerTicks(context.Context) ([]model.WorkerTick, error) {
	return nil, nil
}

func (s *recordingScheduleStore) RecordWorkerTick(context.Context, string, time.Time) error {
	return nil
}

func (s *recordingScheduleStore) RecordWorkerOutcome(_ context.Context, job string, count int64, errText string) error {
	s.outcomes = append(s.outcomes, recordedOutcome{job: job, count: count, err: errText})
	return nil
}

func runExecuteJob(t *testing.T, ctx context.Context, run func(context.Context) (int, error)) recordedOutcome {
	t.Helper()

	store := &recordingScheduleStore{}
	job := scheduledJob{name: "test-job", run: run}

	executeJob(ctx, job, service.NewWorkerSchedule(store), service.NewWorkerActivity())

	if len(store.outcomes) != 1 {
		t.Fatalf("expected exactly one recorded outcome, got %d", len(store.outcomes))
	}

	return store.outcomes[0]
}

func TestExecuteJobRecordsCount(t *testing.T) {
	got := runExecuteJob(t, context.Background(), func(context.Context) (int, error) {
		return 7, nil
	})

	if got.count != 7 || got.err != "" {
		t.Fatalf("recorded outcome = %+v, want count 7 and no error", got)
	}
}

func TestExecuteJobRecordsError(t *testing.T) {
	got := runExecuteJob(t, context.Background(), func(context.Context) (int, error) {
		return 0, errors.New("boom")
	})

	if got.err != "boom" {
		t.Fatalf("recorded outcome = %+v, want error %q", got, "boom")
	}
}

func TestExecuteJobInterruptedRunRecordsCleanRow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := runExecuteJob(t, ctx, func(context.Context) (int, error) {
		return 3, context.Canceled
	})

	if got.count != 0 || got.err != "" {
		t.Fatalf("interrupted run recorded %+v, want a clean row (count 0, no error)", got)
	}
}
