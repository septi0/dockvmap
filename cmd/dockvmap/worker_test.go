package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
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

type failingScheduleStore struct{ recordingScheduleStore }

func (failingScheduleStore) GetWorkerTick(ctx context.Context, _ string) (time.Time, bool, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, false, err
	}

	return time.Time{}, false, errors.New("database is locked")
}

func TestFirstRunDelayLogsOnlyRealScheduleErrors(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	job := scheduledJob{name: "test-job", interval: time.Hour}
	worker := service.NewWorker(&failingScheduleStore{})

	stopping, cancel := context.WithCancel(context.Background())
	cancel()

	if got := firstRunDelay(stopping, job, worker, time.Second); got != time.Hour+time.Second {
		t.Errorf("delay while stopping = %v, want a full interval", got)
	}

	if logs.Len() != 0 {
		t.Errorf("logged while stopping: %q", logs.String())
	}

	firstRunDelay(context.Background(), job, worker, time.Second)

	if !strings.Contains(logs.String(), "reading worker schedule failed") {
		t.Errorf("a real schedule error wasn't logged: %q", logs.String())
	}
}

func runExecuteJob(t *testing.T, ctx context.Context, run func(context.Context) (int, error)) []recordedRun {
	t.Helper()

	store := &recordingScheduleStore{}
	job := scheduledJob{name: "test-job", run: run}

	executeJob(ctx, job, service.NewWorker(store))

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

type recordingMailer struct{ sent int }

func (m *recordingMailer) Send(context.Context, []string, string, string, string) error {
	m.sent++

	return nil
}

type stubWebhook struct{}

func (stubWebhook) Send(context.Context, string, []byte) error { return nil }

type stubFailures struct{}

func (stubFailures) Record(context.Context, service.FailureSource, string, error) {}

type pendingEventStore struct{ events []model.TagNotificationEvent }

func (s *pendingEventStore) ListPendingTagNotificationEvents(context.Context) ([]model.TagNotificationEvent, error) {
	return s.events, nil
}

func (s *pendingEventStore) MarkTagsEventNotified(context.Context, int64) error {
	s.events = nil

	return nil
}

type singleUserStore struct{}

func (singleUserStore) ListUsers(context.Context) ([]model.User, error) {
	user := model.User{Email: "all@example.com"}
	user.Preferences.NotifyLevel = model.NotifyLevelAll

	return []model.User{user}, nil
}

func TestTagNotificationsWaitForTheSweep(t *testing.T) {
	events := &pendingEventStore{events: []model.TagNotificationEvent{{
		ImageEvent: model.ImageEvent{ID: 1, ImageID: 1, ImageName: "nginx", Type: service.EventTypeUpgradeAvailable, Data: model.TagsEventData{Tags: []string{"1.27.4"}}},
	}}}
	mail := &recordingMailer{}

	notifications, err := service.NewNotifications(events, singleUserStore{}, mail, true, stubWebhook{}, nil, stubFailures{})

	if err != nil {
		t.Fatalf("NewNotifications: %v", err)
	}

	worker := service.NewWorker(nil)
	deps := workerDeps{worker: worker, notifications: notifications}

	worker.Begin(service.WorkerJobTagRefresh)

	if sent, err := runTagNotifications(context.Background(), deps); err != nil || sent != 0 || mail.sent != 0 {
		t.Fatalf("during sweep: sent = %d, emails = %d, err = %v", sent, mail.sent, err)
	}

	worker.End(service.WorkerJobTagRefresh)

	if sent, err := runTagNotifications(context.Background(), deps); err != nil || sent != 1 || mail.sent != 1 {
		t.Fatalf("after sweep: sent = %d, emails = %d, err = %v", sent, mail.sent, err)
	}
}

func TestProxyMetricsFlushSavesUsageWithoutCounterChanges(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	tokenID, err := db.CreateProxyToken(ctx, "ci", "hash", false)

	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	usage := service.NewProxyUsage(db)
	flusher := &proxyMetricsFlusher{metrics: proxy.NewMetrics(), history: service.NewProxyMetricsHistory(db), usage: usage}

	usage.RecordTokenUse(tokenID)

	if err := flusher.flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	tokens, err := db.ListProxyTokens(ctx)

	if err != nil || len(tokens) != 1 || tokens[0].LastUsedAt == nil {
		t.Fatalf("token usage not saved: %+v, err = %v", tokens, err)
	}
}
