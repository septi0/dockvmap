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
	RecordWorkerRun(ctx context.Context, job string, at time.Time, count int64, errText string) error
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

func (w *WorkerSchedule) RecordRun(ctx context.Context, job string, count int64, err error) error {
	errText := ""

	if err != nil {
		errText = err.Error()
	}

	return w.store.RecordWorkerRun(ctx, job, time.Now().UTC(), count, errText)
}
