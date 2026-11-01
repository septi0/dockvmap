package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func testDigest(c string) string { return "sha256:" + strings.Repeat(c, 64) }

type fakeDigests struct {
	mu      sync.Mutex
	digests map[string]string
	fail    map[string]error
	lookups []string
}

func (d *fakeDigests) ManifestDigest(_ context.Context, _, repository, reference string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := repository + ":" + reference
	d.lookups = append(d.lookups, key)

	if err := d.fail[key]; err != nil {
		return "", err
	}

	return d.digests[key], nil
}

func (d *fakeDigests) set(key, digest string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.digests[key] = digest
}

type recordingFailures struct {
	mu      sync.Mutex
	entries []string
}

func (r *recordingFailures) Record(_ context.Context, source FailureSource, detail string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, string(source)+" "+detail+": "+err.Error())
}

func newDigestFixture(t *testing.T) (*groupFixture, *fakeDigests, *recordingFailures) {
	f := newGroupFixture(t)
	digests := &fakeDigests{digests: map[string]string{}, fail: map[string]error{}}
	failures := &recordingFailures{}
	f.images.digests = digests
	f.images.failures = failures

	return f, digests, failures
}

func digestOf(image model.Image) string {
	if image.CurrentDigest == nil {
		return ""
	}

	return *image.CurrentDigest
}

func waitForDigest(t *testing.T, f *groupFixture, id int64, tag, digest string) model.Image {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for {
		image := f.image(id)

		if image.CurrentDigestTag != nil && *image.CurrentDigestTag == tag && digestOf(image) == digest {
			return image
		}

		if time.Now().After(deadline) {
			t.Fatalf("image %d digest = %q for tag %v, want %q for %q", id, digestOf(image), image.CurrentDigestTag, digest, tag)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestDigestIsStoredForTheSelectedTagOnly(t *testing.T) {
	f, digests, _ := newDigestFixture(t)
	digests.set("stack/web:1.0.0", testDigest("a"))
	digests.set("stack/web:1.1.0", testDigest("b"))

	id := f.addImage("web", "1.0.0", "1.0.0", "1.1.0")
	got := f.image(id)

	if digestOf(got) != testDigest("a") || *got.CurrentDigestTag != "1.0.0" || got.CurrentDigestSince == nil {
		t.Fatalf("after create: %+v", got)
	}

	if len(digests.lookups) != 1 || digests.lookups[0] != "stack/web:1.0.0" {
		t.Fatalf("lookups = %v, want only the selected tag", digests.lookups)
	}
}

func TestDigestCheckRunsEvenWhenTagNamesAreUnchanged(t *testing.T) {
	f, digests, _ := newDigestFixture(t)
	digests.set("stack/web:latest", testDigest("a"))

	id := f.addImage("web", "latest", "latest")
	first := f.image(id)

	f.refresh(id, "latest")

	if same := f.image(id); !same.CurrentDigestSince.Equal(*first.CurrentDigestSince) || !same.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("an unchanged digest must not be rewritten: before %+v, after %+v", first, same)
	}

	digests.set("stack/web:latest", testDigest("b"))
	time.Sleep(1100 * time.Millisecond)
	f.refresh(id, "latest")

	rebuilt := f.image(id)

	if digestOf(rebuilt) != testDigest("b") || !rebuilt.CurrentDigestSince.After(*first.CurrentDigestSince) {
		t.Fatalf("a rebuilt tag must update the digest and its since: %+v", rebuilt)
	}

	if !rebuilt.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("updated_at changed from %v to %v", first.UpdatedAt, rebuilt.UpdatedAt)
	}
}

func TestDigestMissingHeaderIsStoredAsUnknown(t *testing.T) {
	f, digests, failures := newDigestFixture(t)
	digests.set("stack/web:1.0.0", testDigest("a"))

	id := f.addImage("web", "1.0.0", "1.0.0")
	digests.set("stack/web:1.0.0", "")
	f.refresh(id, "1.0.0")

	got := f.image(id)

	if got.CurrentDigest != nil || got.CurrentDigestSince != nil || *got.CurrentDigestTag != "1.0.0" {
		t.Fatalf("missing digest should be stored as unknown: %+v", got)
	}

	if len(failures.entries) != 0 {
		t.Fatalf("a missing digest is not a failure: %v", failures.entries)
	}
}

func TestDigestLookupFailureKeepsTagCheckAndLastDigest(t *testing.T) {
	f, digests, failures := newDigestFixture(t)
	digests.set("stack/web:1.0.0", testDigest("a"))

	id := f.addImage("web", "1.0.0", "1.0.0")
	digests.fail["stack/web:1.0.0"] = errors.New("registry returned 503")

	f.refresh(id, "1.0.0", "1.1.0")

	got := f.image(id)

	if digestOf(got) != testDigest("a") {
		t.Fatalf("a failed lookup must keep the last digest: %+v", got)
	}

	if got.LastCheckError != nil {
		t.Fatalf("a failed lookup must not fail the tag check: %v", *got.LastCheckError)
	}

	tags, err := f.store.GetImageTags(f.ctx, id)

	if err != nil || len(tags) != 2 {
		t.Fatalf("tags were not refreshed: %v %v", tags, err)
	}

	if len(failures.entries) != 1 || failures.entries[0] != "digest web: registry returned 503" {
		t.Fatalf("failures = %v", failures.entries)
	}
}

func TestDigestFollowsTagChanges(t *testing.T) {
	f, digests, _ := newDigestFixture(t)
	digests.set("stack/a:1.0.0", testDigest("a"))
	digests.set("stack/b:1.0.0", testDigest("b"))
	digests.set("stack/a:1.1.0", testDigest("c"))
	digests.set("stack/b:1.1.0", testDigest("d"))

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	f.group("stack", "1.0.0", a, b)

	if err := f.images.UpdateTag(f.ctx, a, "1.1.0", model.TagHistorySourceManual); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}

	waitForDigest(t, f, a, "1.1.0", testDigest("c"))
	waitForDigest(t, f, b, "1.1.0", testDigest("d"))

	c := f.addImage("c", "1.0.0", "1.0.0", "1.1.0")
	digests.set("stack/c:1.1.0", testDigest("e"))

	if err := f.images.UpdateGroup(f.ctx, *f.image(a).GroupID, ImageGroupInput{Name: "stack", ImageIDs: []int64{a, b, c}, Tag: "1.1.0"}); err != nil {
		t.Fatalf("adding to group: %v", err)
	}

	waitForDigest(t, f, c, "1.1.0", testDigest("e"))
}

func TestDigestForAnOldTagIsNeverStored(t *testing.T) {
	f, digests, _ := newDigestFixture(t)
	digests.set("stack/web:1.0.0", testDigest("a"))

	id := f.addImage("web", "1.0.0", "1.0.0", "1.1.0")

	stale := testDigest("b")

	if err := f.store.UpdateImageDigest(f.ctx, id, "1.1.0", &stale, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if got := f.image(id); digestOf(got) != testDigest("a") || *got.CurrentDigestTag != "1.0.0" {
		t.Fatalf("a digest for a tag the image is not on was stored: %+v", got)
	}
}

type deadlineDigests struct {
	mu        sync.Mutex
	remaining []time.Duration
}

func (d *deadlineDigests) ManifestDigest(ctx context.Context, _, _, _ string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	remaining := time.Duration(-1)

	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}

	d.remaining = append(d.remaining, remaining)

	return testDigest("a"), nil
}

func (d *deadlineDigests) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.remaining)
}

func TestDigestLookupIsCappedInTheSweepAndAfterRetags(t *testing.T) {
	f := newGroupFixture(t)
	digests := &deadlineDigests{}
	f.images.digests = digests

	id := f.addImage("web", "1.0.0", "1.0.0", "1.1.0")

	if err := f.images.UpdateTag(f.ctx, id, "1.1.0", model.TagHistorySourceManual); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}

	waitForDigest(t, f, id, "1.1.0", testDigest("a"))

	if digests.count() != 2 {
		t.Fatalf("lookups = %d, want one on create and one after the retag", digests.count())
	}

	for i, remaining := range digests.remaining {
		if remaining <= 0 || remaining > digestLookupTimeout {
			t.Fatalf("lookup %d had %v left, want at most %v", i, remaining, digestLookupTimeout)
		}
	}
}
