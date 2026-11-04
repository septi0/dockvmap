package service

import (
	"context"
	"slices"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

type listedTags []string

func (l listedTags) ListTags(context.Context, string, string) ([]string, error) {
	return l, nil
}

func TestRefreshReportsACurrentTagThatIsGone(t *testing.T) {
	f, digests, failures := newDigestFixture(t)
	id := f.addImage("web", "v1", "v1", "v2")
	want := "tag v1 no longer exists in the registry; pick another tag"

	refresh := func(tags ...string) *string {
		t.Helper()

		f.images.tagLister = listedTags(tags)

		if err := f.images.RefreshAvailableTags(f.ctx, id, RefreshTagsOpts{RegisterEvent: EventSilent}); err != nil {
			t.Fatalf("refreshing with %v: %v", tags, err)
		}

		return f.image(id).LastCheckError
	}

	lookups := len(digests.lookups)

	for _, round := range []string{"tags changed", "tags unchanged"} {
		if got := refresh("v2", "v3"); got == nil || *got != want {
			t.Errorf("%s: check error = %v, want %q", round, got, want)
		}
	}

	if len(digests.lookups) != lookups {
		t.Errorf("digest lookups for a tag that is gone: %v", digests.lookups[lookups:])
	}

	if !slices.ContainsFunc(failures.entries, func(e string) bool { return e == "refresh web: "+want }) {
		t.Errorf("failures = %q, want the missing tag", failures.entries)
	}

	if got := refresh("v1", "v2", "v3"); got != nil {
		t.Errorf("check error after the tag is back = %q, want none", *got)
	}
}

func TestPickingAnotherTagClearsTheMissingTagError(t *testing.T) {
	f, _, _ := newDigestFixture(t)
	id := f.addImage("web", "v1", "v1", "v2")
	f.images.tagLister = listedTags{"v2", "v3"}

	if err := f.images.RefreshAvailableTags(f.ctx, id, RefreshTagsOpts{RegisterEvent: EventSilent}); err != nil {
		t.Fatalf("refreshing: %v", err)
	}

	if f.image(id).LastCheckError == nil {
		t.Fatal("no check error for the missing tag")
	}

	if err := f.images.UpdateTag(f.ctx, id, "v2", model.TagHistorySourceManual); err != nil {
		t.Fatalf("picking v2: %v", err)
	}

	waitFor(t, "the check error to clear", func() bool {
		image := f.image(id)
		return image.LastCheckError == nil && image.RefreshStatus == model.RefreshStatusIdle
	})
}
