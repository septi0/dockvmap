package service

import (
	"context"
	"time"
)

const WorkerJobTagRefresh = "image-tag-refresh"

type workerScheduleStore interface {
	GetWorkerTick(ctx context.Context, job string) (time.Time, bool, error)
	RecordWorkerTick(ctx context.Context, job string, at time.Time) error
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

func (w *WorkerSchedule) MarkRun(ctx context.Context, job string) error {
	return w.store.RecordWorkerTick(ctx, job, time.Now().UTC())
}
