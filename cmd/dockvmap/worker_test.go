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

type recordedRun struct {
	job   string
	count int64
	err   string
}

type recordingScheduleStore struct {
	runs []recordedRun
}

func (s *recordingScheduleStore) GetWorkerTick(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (s *recordingScheduleStore) ListWorkerTicks(context.Context) ([]model.WorkerTick, error) {
	return nil, nil
}

func (s *recordingScheduleStore) RecordWorkerRun(_ context.Context, job string, _ time.Time, count int64, errText string) error {
	s.runs = append(s.runs, recordedRun{job: job, count: count, err: errText})
	return nil
}

func runExecuteJob(t *testing.T, ctx context.Context, run func(context.Context) (int, error)) []recordedRun {
	t.Helper()

	store := &recordingScheduleStore{}
	job := scheduledJob{name: "test-job", run: run}

	executeJob(ctx, job, service.NewWorkerSchedule(store), service.NewWorkerActivity())

	return store.runs
}

func TestExecuteJobRecordsCount(t *testing.T) {
	runs := runExecuteJob(t, context.Background(), func(context.Context) (int, error) {
		return 7, nil
	})

	if len(runs) != 1 || runs[0].count != 7 || runs[0].err != "" {
		t.Fatalf("recorded runs = %+v, want one run with count 7 and no error", runs)
	}
}

func TestExecuteJobRecordsError(t *testing.T) {
	runs := runExecuteJob(t, context.Background(), func(context.Context) (int, error) {
		return 0, errors.New("boom")
	})

	if len(runs) != 1 || runs[0].err != "boom" {
		t.Fatalf("recorded runs = %+v, want one run with error %q", runs, "boom")
	}
}

func TestExecuteJobInterruptedRunSkipsWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runs := runExecuteJob(t, ctx, func(context.Context) (int, error) {
		return 3, context.Canceled
	})

	if len(runs) != 0 {
		t.Fatalf("interrupted run recorded %+v, want no write", runs)
	}
}
