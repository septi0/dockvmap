package service

import (
	"context"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

const WorkerJobTagRefresh = "image-tag-refresh"

type workerScheduleStore interface {
	GetWorkerTick(ctx context.Context, job string) (time.Time, bool, error)
	ListWorkerTicks(ctx context.Context) ([]model.WorkerTick, error)
	RecordWorkerTick(ctx context.Context, job string, at time.Time) error
	RecordWorkerOutcome(ctx context.Context, job string, count int64, errText string) error
}

type WorkerSchedule struct {
	store workerScheduleStore
}

func NewWorkerSchedule(store workerScheduleStore) *WorkerSchedule {
	return &WorkerSchedule{store: store}
}

func (w *WorkerSchedule) LastRun(ctx context.Context, job string) (time.Time, bool, error) {
	return w.store.GetWorkerTick(ctx, job)
}

func (w *WorkerSchedule) Ticks(ctx context.Context) ([]model.WorkerTick, error) {
	return w.store.ListWorkerTicks(ctx)
}

func (w *WorkerSchedule) MarkRun(ctx context.Context, job string) error {
	return w.store.RecordWorkerTick(ctx, job, time.Now().UTC())
}

func (w *WorkerSchedule) MarkOutcome(ctx context.Context, job string, count int64, err error) error {
	errText := ""

	if err != nil {
		errText = err.Error()
	}

	return w.store.RecordWorkerOutcome(ctx, job, count, errText)
}
