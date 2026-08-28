package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type FailureSource string

const (
	FailureSourceWebhook           FailureSource = "webhook"
	FailureSourceEmail             FailureSource = "email"
	FailureSourceRefresh           FailureSource = "refresh"
	FailureSourceDiscovery         FailureSource = "discovery"
	FailureSourceEventRegistration FailureSource = "event_registration"
)

type Failure struct {
	Source     FailureSource
	Detail     string
	Error      string
	OccurredAt time.Time
}

const recentFailureLimit = 50

type failureLogStore interface {
	InsertBackgroundFailure(ctx context.Context, source, detail, errText string) error
	ListRecentBackgroundFailures(ctx context.Context, limit int) ([]model.BackgroundFailure, error)
	DeleteBackgroundFailuresBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

type FailureLog struct {
	store failureLogStore
}

func NewFailureLog(store failureLogStore) *FailureLog {
	return &FailureLog{store: store}
}

func (l *FailureLog) Record(ctx context.Context, source FailureSource, detail string, err error) {
	if err == nil {
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if writeErr := l.store.InsertBackgroundFailure(writeCtx, string(source), detail, err.Error()); writeErr != nil {
		slog.Error("recording background failure failed", "source", source, "detail", detail, "error", writeErr)
	}
}

func (l *FailureLog) Recent(ctx context.Context) ([]Failure, error) {
	rows, err := l.store.ListRecentBackgroundFailures(ctx, recentFailureLimit)

	if err != nil {
		return nil, err
	}

	failures := make([]Failure, len(rows))

	for i, row := range rows {
		failures[i] = Failure{
			Source:     FailureSource(row.Source),
			Detail:     row.Detail,
			Error:      row.Error,
			OccurredAt: row.OccurredAt,
		}
	}

	return failures, nil
}

func (l *FailureLog) CleanupOld(ctx context.Context, retention time.Duration) (int64, error) {
	return l.store.DeleteBackgroundFailuresBefore(ctx, time.Now().UTC().Add(-retention))
}
