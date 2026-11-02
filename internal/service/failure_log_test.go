package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

func TestFailureLogLatestDistinctCollapsesRepeats(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	log := NewFailureLog(st)
	registryDown := errors.New("lookup registry.example.com: no such host")

	log.Record(ctx, FailureSourceRefresh, "myapp", registryDown)
	log.Record(ctx, FailureSourceWebhook, "https://hooks.example.com", errors.New("connection refused"))
	log.Record(ctx, FailureSourceRefresh, "myapp", registryDown)
	log.Record(ctx, FailureSourceRefresh, "other", registryDown)
	log.Record(ctx, FailureSourceRefresh, "myapp", registryDown)

	latest, err := log.LatestDistinct(ctx, 5)

	if err != nil {
		t.Fatalf("LatestDistinct: %v", err)
	}

	got := make([]string, len(latest))

	for i, f := range latest {
		got[i] = f.Detail
	}

	if len(latest) != 3 || got[0] != "myapp" || got[1] != "other" || got[2] != "https://hooks.example.com" {
		t.Fatalf("details = %v, want myapp, other, webhook newest first", got)
	}

	if latest[0].Occurrences != 3 || latest[1].Occurrences != 1 || latest[2].Occurrences != 1 {
		t.Fatalf("occurrences = %d, %d, %d, want 3, 1, 1", latest[0].Occurrences, latest[1].Occurrences, latest[2].Occurrences)
	}

	all, err := log.List(ctx, model.BackgroundFailureListFilters{Pagination: model.Pagination{Limit: 10}})

	if err != nil || len(all) != 5 || all[0].Occurrences != 1 {
		t.Fatalf("full log = %d rows (err %v), want all 5 kept with one occurrence each", len(all), err)
	}
}
