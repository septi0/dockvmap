package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
)

const followerPullReportEvery = time.Minute

// pullReporter tells the primary about pulls served here, so its pull tracking stays true; only its own goroutine touches it
type pullReporter struct {
	images    map[int64]time.Time
	tokens    map[int64]time.Time
	lastError string
}

// runPullReports sends what changed since the last report the primary took; while it's down nothing is marked sent, so the next one catches up
func (f *Follower) runPullReports(ctx context.Context) {
	reporter := &pullReporter{images: make(map[int64]time.Time), tokens: make(map[int64]time.Time)}
	ticker := time.NewTicker(followerPullReportEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		f.reportPulls(ctx, reporter)
	}
}

func (f *Follower) reportPulls(ctx context.Context, reporter *pullReporter) {
	f.mu.Lock()
	synced := f.latest != nil
	f.mu.Unlock()

	// row ids only match the primary's while following it
	if !synced || f.isStopped() {
		return
	}

	images, tokens, err := f.store.ProxyUsage(ctx)

	if err != nil {
		slog.Error("reading pulls to report failed", "error", err)
		return
	}

	report := replicationapi.PullReport{
		Images: changedSince(images, reporter.images, func(u model.ImageUsage) (int64, time.Time) { return u.ImageID, u.PulledAt }),
		Tokens: changedSince(tokens, reporter.tokens, func(u model.ProxyTokenUsage) (int64, time.Time) { return u.TokenID, u.UsedAt }),
	}

	if len(report.Images) == 0 && len(report.Tokens) == 0 {
		return
	}

	if err := f.client.ReportPulls(ctx, report); err != nil {
		if ctx.Err() == nil && err.Error() != reporter.lastError {
			slog.Warn("reporting pulls to the primary failed; they're sent once it answers", "error", err)
		}

		reporter.lastError = err.Error()

		return
	}

	reporter.lastError = ""

	for _, u := range report.Images {
		reporter.images[u.ImageID] = u.PulledAt
	}

	for _, u := range report.Tokens {
		reporter.tokens[u.TokenID] = u.UsedAt
	}
}

func changedSince[T any](items []T, sent map[int64]time.Time, key func(T) (int64, time.Time)) []T {
	var changed []T

	for _, item := range items {
		if id, at := key(item); !at.Equal(sent[id]) {
			changed = append(changed, item)
		}
	}

	return changed
}
