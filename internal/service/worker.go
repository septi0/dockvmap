package service

import (
	"context"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

const WorkerJobTagRefresh = "image-tag-refresh"

type WorkerJobDescriptor struct {
	Name           string
	Description    string
	Interval       time.Duration
	Enabled        bool
	DisabledReason string
	Triggerable    bool
}

type workerScheduleStore interface {
	GetWorkerTick(ctx context.Context, job string) (time.Time, bool, error)
	ListWorkerTicks(ctx context.Context) ([]model.WorkerTick, error)
	RecordWorkerRun(ctx context.Context, job string, at time.Time, count int64, errText string) error
}

type Worker struct {
	store workerScheduleStore

	activityMu sync.RWMutex
	activity   map[string]bool

	triggerMu sync.Mutex
	triggers  map[string]chan struct{}

	catalogMu sync.RWMutex
	catalog   []WorkerJobDescriptor
}

func NewWorker(store workerScheduleStore) *Worker {
	return &Worker{
		store:    store,
		activity: make(map[string]bool),
		triggers: make(map[string]chan struct{}),
	}
}

func (w *Worker) LastRun(ctx context.Context, job string) (time.Time, bool, error) {
	return w.store.GetWorkerTick(ctx, job)
}

func (w *Worker) Ticks(ctx context.Context) ([]model.WorkerTick, error) {
	return w.store.ListWorkerTicks(ctx)
}

func (w *Worker) RecordRun(ctx context.Context, job string, count int64, err error) error {
	errText := ""

	if err != nil {
		errText = err.Error()
	}

	return w.store.RecordWorkerRun(ctx, job, time.Now().UTC(), count, errText)
}

func (w *Worker) Register(job string) <-chan struct{} {
	w.triggerMu.Lock()
	defer w.triggerMu.Unlock()

	ch, ok := w.triggers[job]

	if !ok {
		ch = make(chan struct{}, 1)
		w.triggers[job] = ch
	}

	return ch
}

func (w *Worker) Trigger(job string) bool {
	w.triggerMu.Lock()
	ch, ok := w.triggers[job]
	w.triggerMu.Unlock()

	if !ok {
		return false
	}

	select {
	case ch <- struct{}{}:
	default:
	}

	return true
}

func (w *Worker) Begin(job string) { w.setActivity(job, true) }

func (w *Worker) End(job string) { w.setActivity(job, false) }

func (w *Worker) Running(job string) bool {
	w.activityMu.RLock()
	defer w.activityMu.RUnlock()

	return w.activity[job]
}

func (w *Worker) setActivity(job string, running bool) {
	w.activityMu.Lock()
	defer w.activityMu.Unlock()

	w.activity[job] = running
}

func (w *Worker) SetCatalog(jobs []WorkerJobDescriptor) {
	w.catalogMu.Lock()
	defer w.catalogMu.Unlock()

	w.catalog = jobs
}

func (w *Worker) Catalog() []WorkerJobDescriptor {
	w.catalogMu.RLock()
	defer w.catalogMu.RUnlock()

	return w.catalog
}
