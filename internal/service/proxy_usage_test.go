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

func (s *failingUsageStore) RecordProxyUsage(context.Context, []model.ImageUsage, []model.ImageDigest, []model.ProxyTokenUsage) error {
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

func TestProxyUsageKeepsTheLatestPull(t *testing.T) {
	usage, clock := newTestProxyUsage(&failingUsageStore{})

	usage.RecordImagePull(1, "v2.6")
	clock.advance(time.Minute)
	usage.RecordImagePull(1, "v2.7")

	got := applied(usage, 1)

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(clock.at) || *got.LastPulledTag != "v2.7" {
		t.Fatalf("pulled = %v %v, want %v v2.7", got.LastPulledAt, got.LastPulledTag, clock.at)
	}

	if other := applied(usage, 2); other.LastPulledAt != nil {
		t.Fatalf("unrecorded image got usage: %+v", other)
	}
}

func TestProxyUsageOverlayKeepsStoredValuesWhenNothingIsPending(t *testing.T) {
	usage, _ := newTestProxyUsage(&failingUsageStore{})
	stored := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tag := "v2.6"

	images := []model.Image{{ID: 1, LastPulledAt: &stored, LastPulledTag: &tag}}
	usage.ApplyToImages(images)

	if images[0].LastPulledAt != &stored || images[0].LastPulledTag != &tag {
		t.Fatalf("stored values were replaced: %+v", images[0])
	}
}

func TestProxyUsageFlushPersistsAndClears(t *testing.T) {
	f := newGroupFixture(t)
	id := f.addImage("nextcloud", "v1", "v1")
	other := f.addImage("redis", "v1", "v1")

	tokenID, err := f.store.CreateProxyToken(f.ctx, "ci", "hash", false)

	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	before := f.image(id)
	usage, clock := newTestProxyUsage(f.store)

	usage.RecordImagePull(id, "v1")
	usage.RecordTokenUse(tokenID)
	usage.RecordImagePull(999, "gone")

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := f.image(id)

	if got.LastPulledAt == nil || !got.LastPulledAt.Equal(clock.at) || *got.LastPulledTag != "v1" {
		t.Fatalf("stored usage = %+v", got)
	}

	if !got.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at changed from %v to %v", before.UpdatedAt, got.UpdatedAt)
	}

	if untouched := f.image(other); untouched.LastPulledAt != nil {
		t.Fatalf("image without a pull = %+v", untouched)
	}

	tokens, err := f.store.ListProxyTokens(f.ctx)

	if err != nil || len(tokens) != 1 || tokens[0].LastUsedAt == nil || !tokens[0].LastUsedAt.Equal(clock.at) {
		t.Fatalf("tokens = %+v, err = %v", tokens, err)
	}

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("empty flush: %v", err)
	}
}

func TestProxyUsageFailedFlushKeepsEntries(t *testing.T) {
	store := &failingUsageStore{}
	usage, clock := newTestProxyUsage(store)

	usage.RecordImagePull(1, "v2.7")
	usage.RecordImagePull(2, "v1.0")
	usage.RecordTagDigest(3, "v1", testDigest("b"))
	usage.RecordTokenUse(5)

	store.during = func() {
		clock.advance(time.Minute)
		usage.RecordImagePull(1, "v2.8")
	}

	if err := usage.Flush(context.Background()); err == nil {
		t.Fatal("flush should report the store error")
	}

	if got := applied(usage, 1); got.LastPulledAt == nil || !got.LastPulledAt.Equal(clock.at) || *got.LastPulledTag != "v2.8" {
		t.Fatalf("newer pull lost: %+v", got)
	}

	if got := applied(usage, 2); got.LastPulledAt == nil || *got.LastPulledTag != "v1.0" {
		t.Fatalf("pull from the failed flush lost: %+v", got)
	}

	pending := model.Image{ID: 3, Tag: "v1"}
	usage.ApplyTagDigest(&pending)

	if pending.CurrentDigest == nil || *pending.CurrentDigest != testDigest("b") {
		t.Fatalf("digest from the failed flush lost: %+v", pending)
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
				usage.RecordImagePull(id, "v1")
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

	if got := f.image(id); got.LastPulledAt == nil {
		t.Fatalf("usage not stored: %+v", got)
	}
}

type recordingTokenUsage struct{ ids []int64 }

func (r *recordingTokenUsage) RecordTokenUse(id int64) { r.ids = append(r.ids, id) }

func TestProxyTokenVerifyRecordsUseOnlyForValidTokens(t *testing.T) {
	f := newGroupFixture(t)
	used := &recordingTokenUsage{}
	tokens := NewProxyTokens(f.store, f.audit, used)

	id, secret, err := tokens.Create(f.ctx, "ci", true)

	if err != nil {
		t.Fatalf("creating token: %v", err)
	}

	if token, err := tokens.Verify(f.ctx, "wrong"); token != nil || err != nil {
		t.Fatalf("wrong token: token = %v, err = %v", token, err)
	}

	if token, err := tokens.Verify(f.ctx, ""); token != nil || err != nil {
		t.Fatalf("empty token: token = %v, err = %v", token, err)
	}

	if len(used.ids) != 0 {
		t.Fatalf("invalid tokens recorded use: %v", used.ids)
	}

	token, err := tokens.Verify(f.ctx, secret)

	if err != nil || token == nil || token.ID != id || token.Label != "ci" || !token.CanPush {
		t.Fatalf("valid token: token = %+v, err = %v", token, err)
	}

	if len(used.ids) != 1 || used.ids[0] != id {
		t.Fatalf("recorded = %v, want [%d]", used.ids, id)
	}
}

func TestProxyUsageTagDigestOverlay(t *testing.T) {
	usage, clock := newTestProxyUsage(&failingUsageStore{})
	stored := testDigest("a")
	storedTag := "v1"
	storedSince := clock.at.Add(-48 * time.Hour)

	image := func(tag string) model.Image {
		return model.Image{ID: 1, Tag: tag, CurrentDigest: &stored, CurrentDigestTag: &storedTag, CurrentDigestSince: &storedSince}
	}

	usage.RecordTagDigest(1, "v1", testDigest("b"))
	firstSeen := clock.at
	clock.advance(time.Minute)
	usage.RecordTagDigest(1, "v1", testDigest("b"))

	images := []model.Image{image("v1")}
	usage.ApplyToImages(images)

	if digest, since := images[0].TagDigest(); digest == nil || *digest != testDigest("b") || !since.Equal(firstSeen) {
		t.Fatalf("overlay = %v since %v, want %s since %v", digest, since, testDigest("b"), firstSeen)
	}

	retagged := image("v2")
	usage.ApplyTagDigest(&retagged)

	if retagged.CurrentDigest != &stored {
		t.Fatalf("a digest seen for another tag was applied: %+v", retagged)
	}

	usage.RecordTagDigest(1, "v1", stored)
	same := image("v1")
	usage.ApplyTagDigest(&same)

	if same.CurrentDigestSince != &storedSince {
		t.Fatalf("the stored first seen time was replaced for the same digest: %+v", same)
	}
}

func TestProxyUsageFlushSavesTheServedDigest(t *testing.T) {
	f := newGroupFixture(t)
	id := f.addImage("nextcloud", "v1", "v1")
	usage, clock := newTestProxyUsage(f.store)
	stored := testDigest("a")

	if err := f.store.UpdateImageDigest(f.ctx, id, "v1", &stored, clock.at.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	usage.RecordTagDigest(id, "v1", testDigest("b"))
	usage.RecordTagDigest(id+1, "v1", testDigest("c"))

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if digest, since := f.image(id).TagDigest(); digest == nil || *digest != testDigest("b") || !since.Equal(clock.at) {
		t.Fatalf("stored digest = %v since %v", digest, since)
	}

	usage.RecordTagDigest(id, "v0", testDigest("d"))

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if digest, _ := f.image(id).TagDigest(); digest == nil || *digest != testDigest("b") {
		t.Fatalf("a digest seen for another tag was saved: %v", digest)
	}
}
