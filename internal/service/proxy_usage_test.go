package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type stepClock struct{ at time.Time }

func (c *stepClock) now() time.Time { return c.at }

func (c *stepClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newTestProxyUsage(store proxyUsageStore) (*ProxyUsage, *stepClock) {
	clock := &stepClock{at: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	usage := NewProxyUsage(store)
	usage.now = clock.now

	return usage, clock
}

type failingUsageStore struct {
	during func()
	calls  int
}

func (s *failingUsageStore) RecordProxyUsage(context.Context, []model.ImageUsage, []model.ProxyTokenUsage) error {
	s.calls++

	if s.during != nil {
		s.during()
	}

	return errors.New("database is locked")
}

func applied(usage *ProxyUsage, id int64) model.Image {
	images := []model.Image{{ID: id}}
	usage.ApplyToImages(images)

	return images[0]
}

func TestProxyUsageSeparatesRequestsFromPulls(t *testing.T) {
	usage, clock := newTestProxyUsage(&failingUsageStore{})
	pulledAt := clock.at

	usage.RecordImageRequest(1, "v2.7", true)
	clock.advance(time.Minute)
	usage.RecordImageRequest(1, "v2.7", false)

	got := applied(usage, 1)

	if got.LastRequestedAt == nil || !got.LastRequestedAt.Equal(clock.at) {
		t.Fatalf("requested at = %v, want %v", got.LastRequestedAt, clock.at)
	}

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(pulledAt) || *got.LastPulledTag != "v2.7" {
		t.Fatalf("pulled = %v %v, want %v v2.7", got.LastPulledAt, got.LastPulledTag, pulledAt)
	}

	if other := applied(usage, 2); other.LastRequestedAt != nil || other.LastPulledAt != nil {
		t.Fatalf("unrecorded image got usage: %+v", other)
	}
}

func TestProxyUsageRequestOnlyHasNoPull(t *testing.T) {
	usage, _ := newTestProxyUsage(&failingUsageStore{})

	usage.RecordImageRequest(1, "v2.7", false)

	if got := applied(usage, 1); got.LastRequestedAt == nil || got.LastPulledAt != nil || got.LastPulledTag != nil {
		t.Fatalf("got %+v, want a request and no pull", got)
	}
}

func TestProxyUsageOverlayKeepsStoredValuesWhenNothingIsPending(t *testing.T) {
	usage, _ := newTestProxyUsage(&failingUsageStore{})
	stored := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tag := "v2.6"

	images := []model.Image{{ID: 1, LastPulledAt: &stored, LastPulledTag: &tag, LastRequestedAt: &stored}}
	usage.ApplyToImages(images)

	if images[0].LastPulledAt != &stored || images[0].LastPulledTag != &tag {
		t.Fatalf("stored values were replaced: %+v", images[0])
	}
}

func TestProxyUsageFlushPersistsAndClears(t *testing.T) {
	f := newGroupFixture(t)
	id := f.addImage("nextcloud", "v1", "v1")
	other := f.addImage("redis", "v1", "v1")

	tokenID, err := f.store.CreateProxyToken(f.ctx, "ci", "hash")

	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	before := f.image(id)
	usage, clock := newTestProxyUsage(f.store)

	usage.RecordImageRequest(id, "v1", true)
	usage.RecordImageRequest(other, "v1", false)
	usage.RecordTokenUse(tokenID)
	usage.RecordImageRequest(999, "gone", true)

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := f.image(id)

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(clock.at) || *got.LastPulledTag != "v1" || !got.LastRequestedAt.Equal(clock.at) {
		t.Fatalf("stored usage = %+v", got)
	}

	if !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at changed from %v to %v", before.UpdatedAt, got.UpdatedAt)
	}

	if requestOnly := f.image(other); requestOnly.LastRequestedAt == nil || requestOnly.LastPulledAt != nil {
		t.Fatalf("request-only image = %+v", requestOnly)
	}

	tokens, err := f.store.ListProxyTokens(f.ctx)

	if err != nil || len(tokens) != 1 || tokens[0].LastUsedAt == nil || !tokens[0].LastUsedAt.Equal(clock.at) {
		t.Fatalf("tokens = %+v, err = %v", tokens, err)
	}

	clock.advance(time.Hour)
	usage.RecordImageRequest(id, "v1", false)

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("second flush: %v", err)
	}

	if kept := f.image(id); kept.LastPulledAt == nil || !kept.LastRequestedAt.Equal(clock.at) || kept.LastRequestedAt.Equal(*kept.LastPulledAt) {
		t.Fatalf("a request without a pull must keep the earlier pull: %+v", kept)
	}

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("empty flush: %v", err)
	}
}

func TestProxyUsageFailedFlushKeepsEntries(t *testing.T) {
	store := &failingUsageStore{}
	usage, clock := newTestProxyUsage(store)
	pulledAt := clock.at

	usage.RecordImageRequest(1, "v2.7", true)
	usage.RecordTokenUse(5)

	store.during = func() {
		clock.advance(time.Minute)
		usage.RecordImageRequest(1, "v2.7", false)
	}

	if err := usage.Flush(context.Background()); err == nil {
		t.Fatal("flush should report the store error")
	}

	got := applied(usage, 1)

	if !got.LastRequestedAt.Equal(clock.at) {
		t.Fatalf("newer request lost: %v", got.LastRequestedAt)
	}

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(pulledAt) || *got.LastPulledTag != "v2.7" {
		t.Fatalf("pull from the failed flush lost: %+v", got)
	}

	tokens := []model.ProxyToken{{ID: 5}}
	usage.ApplyToTokens(tokens)

	if tokens[0].LastUsedAt == nil {
		t.Fatal("token use from the failed flush lost")
	}

	store.during = nil

	if err := usage.Flush(context.Background()); err == nil || store.calls != 2 {
		t.Fatalf("retry should flush again, calls = %d", store.calls)
	}
}

func TestProxyUsageIsSafeForConcurrentUse(t *testing.T) {
	f := newGroupFixture(t)
	id := f.addImage("nextcloud", "v1", "v1")
	usage := NewProxyUsage(f.store)

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 200 {
				usage.RecordImageRequest(id, "v1", i%2 == 0)
				usage.RecordTokenUse(int64(i))
				usage.ApplyToImages([]model.Image{{ID: id}})
			}
		}()
	}

	for range 5 {
		if err := usage.Flush(f.ctx); err != nil {
			t.Fatalf("flush: %v", err)
		}
	}

	wg.Wait()

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("final flush: %v", err)
	}

	if got := f.image(id); got.LastRequestedAt == nil || got.LastPulledAt == nil {
		t.Fatalf("usage not stored: %+v", got)
	}
}

type recordingTokenUsage struct{ ids []int64 }

func (r *recordingTokenUsage) RecordTokenUse(id int64) { r.ids = append(r.ids, id) }

func TestProxyTokenVerifyRecordsUseOnlyForValidTokens(t *testing.T) {
	f := newGroupFixture(t)
	used := &recordingTokenUsage{}
	tokens := NewProxyTokens(f.store, f.audit, used)

	id, secret, err := tokens.Create(f.ctx, "ci")

	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	if ok, err := tokens.Verify(f.ctx, "wrong"); ok || err != nil {
		t.Fatalf("wrong token: ok = %v, err = %v", ok, err)
	}

	if ok, err := tokens.Verify(f.ctx, ""); ok || err != nil {
		t.Fatalf("empty token: ok = %v, err = %v", ok, err)
	}

	if len(used.ids) != 0 {
		t.Fatalf("invalid tokens recorded use: %v", used.ids)
	}

	if ok, err := tokens.Verify(f.ctx, secret); !ok || err != nil {
		t.Fatalf("valid token: ok = %v, err = %v", ok, err)
	}

	if len(used.ids) != 1 || used.ids[0] != id {
		t.Fatalf("recorded = %v, want [%d]", used.ids, id)
	}
}
