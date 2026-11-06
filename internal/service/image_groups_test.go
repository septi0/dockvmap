package service

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

type stubFilter struct{}

func (stubFilter) Apply(tags []string) []string { return tags }

type stubEvents struct{}

func (stubEvents) HandleEvent(context.Context, EventMode, *model.Image, []model.ImageTag, []model.ImageTag, bool, string) error {
	return nil
}

type stubLister struct{}

func (stubLister) ListTags(context.Context, string, string) ([]string, error) {
	return nil, errors.New("upstream is not available in tests")
}

func (stubLister) ManifestDigest(context.Context, string, string, string) (string, error) {
	return "", errors.New("upstream is not available in tests")
}

type stubFailures struct{}

func (stubFailures) Record(context.Context, FailureSource, string, error) {}

type recordingAudit struct {
	mu    sync.Mutex
	types []string
}

func (a *recordingAudit) Record(_ context.Context, auditType string, _ any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.types = append(a.types, auditType)

	return nil
}

func (a *recordingAudit) count(auditType string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0

	for _, t := range a.types {
		if t == auditType {
			count++
		}
	}

	return count
}

type groupFixture struct {
	t          *testing.T
	ctx        context.Context
	store      *store.Store
	images     *Images
	audit      *recordingAudit
	registryID int64
	changed    [][]string
}

func newGroupFixture(t *testing.T) *groupFixture {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { st.Close() })

	ctx := context.Background()

	registryID, err := st.CreateRegistry(ctx, model.Registry{Registry: "registry.test"})

	if err != nil {
		t.Fatalf("creating registry: %v", err)
	}

	audit := &recordingAudit{}

	f := &groupFixture{t: t, ctx: ctx, store: st, audit: audit, registryID: registryID}

	f.images = NewImages(ImagesDeps{
		Store:     st,
		TagLister: stubLister{},
		Digests:   stubLister{},
		Events:    stubEvents{},
		Audit:     audit,
		Failures:  stubFailures{},
		TagFilter: stubFilter{},
		BgCtx:     ctx,
	})

	f.images.OnTagChanged(func(names ...string) {
		f.changed = append(f.changed, names)
	})

	return f
}

func (f *groupFixture) addImage(name, tag string, tags ...string) int64 {
	f.t.Helper()

	err := f.images.Create(f.ctx, model.Image{
		Name:       name,
		RegistryID: f.registryID,
		Repository: "stack/" + name,
		Tag:        tag,
	}, tags)

	if err != nil {
		f.t.Fatalf("creating image %q: %v", name, err)
	}

	image, err := f.store.GetImage(f.ctx, name)

	if err != nil || image == nil {
		f.t.Fatalf("loading image %q: %v", name, err)
	}

	return image.ID
}

func (f *groupFixture) refresh(imageID int64, tags ...string) {
	f.t.Helper()

	opts := RefreshTagsOpts{FlagAsNew: true, RegisterEvent: EventSilent, Tags: tags}

	if err := f.images.RefreshAvailableTags(f.ctx, imageID, opts); err != nil {
		f.t.Fatalf("refreshing image %d: %v", imageID, err)
	}
}

func (f *groupFixture) image(id int64) model.Image {
	f.t.Helper()

	image, err := f.images.GetByID(f.ctx, id)

	if err != nil {
		f.t.Fatalf("getting image %d: %v", id, err)
	}

	return *image
}

func (f *groupFixture) group(name string, tag string, ids ...int64) int64 {
	f.t.Helper()

	id, err := f.images.CreateGroup(f.ctx, ImageGroupInput{Name: name, ImageIDs: ids, Tag: tag})

	if err != nil {
		f.t.Fatalf("creating group %q: %v", name, err)
	}

	return id
}

func (f *groupFixture) tags(id int64, want string) {
	f.t.Helper()

	if got := f.image(id).Tag; got != want {
		f.t.Fatalf("image %d tag = %q, want %q", id, got, want)
	}
}

func TestUpdateTagMovesEveryGroupMember(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	f.group("stack", "1.0.0", a, b)
	f.changed = nil

	if err := f.images.UpdateTag(f.ctx, b, "1.1.0", model.TagHistorySourceManual); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}

	f.tags(a, "1.1.0")
	f.tags(b, "1.1.0")

	for _, id := range []int64{a, b} {
		history, err := f.images.GetTagHistory(f.ctx, id)

		if err != nil {
			t.Fatal(err)
		}

		if len(history) != 2 || history[0].Tag != "1.1.0" {
			t.Fatalf("image %d history = %+v, want the update on top of the created entry", id, history)
		}
	}

	if len(f.changed) != 1 || !slices.Equal(f.changed[0], []string{"a", "b"}) {
		t.Fatalf("tag change notifications = %v", f.changed)
	}

	if got := f.audit.count(AuditTypeImageTagChanged); got != 2 {
		t.Fatalf("tag change audit rows = %d, want 2", got)
	}
}

func TestUpdateTagChangesNothingWhenAMemberLacksTheTag(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0")
	f.group("stack", "1.0.0", a, b)

	err := f.images.UpdateTag(f.ctx, a, "1.1.0", model.TagHistorySourceManual)

	var tagErr *GroupTagUnavailableError

	if !errors.As(err, &tagErr) {
		t.Fatalf("error = %v, want GroupTagUnavailableError", err)
	}

	if !errors.Is(err, ErrTagUnavailable) {
		t.Fatalf("error = %v, want it to wrap ErrTagUnavailable", err)
	}

	if !slices.Equal(tagErr.Images, []string{"b"}) {
		t.Fatalf("missing images = %v, want [b]", tagErr.Images)
	}

	f.tags(a, "1.0.0")
	f.tags(b, "1.0.0")
}

func TestUpdateTagUngroupedKeepsPlainErrorAndBehaviour(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")

	err := f.images.UpdateTag(f.ctx, a, "9.9.9", model.TagHistorySourceManual)

	var tagErr *GroupTagUnavailableError

	if !errors.Is(err, ErrTagUnavailable) || errors.As(err, &tagErr) {
		t.Fatalf("error = %v, want plain ErrTagUnavailable", err)
	}

	if err := f.images.UpdateTag(f.ctx, a, "1.0.0", model.TagHistorySourceManual); err != nil {
		t.Fatalf("same tag must be a no-op: %v", err)
	}

	if len(f.changed) != 0 {
		t.Fatalf("no-op notified a tag change: %v", f.changed)
	}

	if err := f.images.UpdateTag(f.ctx, a, "1.1.0", model.TagHistorySourceManual); err != nil {
		t.Fatal(err)
	}

	f.tags(a, "1.1.0")

	if image := f.image(a); image.UpdateAvailable {
		t.Fatalf("image still flagged after moving to the newest tag: %+v", image)
	}
}

func TestUpdateTagFailsFastWhileAMemberRefreshHoldsItsLock(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	f.group("stack", "1.0.0", a, b)

	unlock := f.images.refreshLocker.lock(b)
	defer unlock()

	ctx, cancel := context.WithTimeout(f.ctx, 150*time.Millisecond)
	defer cancel()

	err := f.images.UpdateTag(ctx, a, "1.1.0", model.TagHistorySourceManual)

	if !errors.Is(err, ErrImageGroupBusy) {
		t.Fatalf("error = %v, want ErrImageGroupBusy", err)
	}

	f.tags(a, "1.0.0")
	f.tags(b, "1.0.0")

	if len(f.images.refreshLocker.locks) != 1 {
		t.Fatalf("locks left behind: %d", len(f.images.refreshLocker.locks))
	}
}

func TestConcurrentGroupUpdatesLeaveMembersOnOneTag(t *testing.T) {
	f := newGroupFixture(t)

	tags := []string{"1.0.0", "1.1.0", "1.2.0"}
	a := f.addImage("a", "1.0.0", tags...)
	b := f.addImage("b", "1.0.0", tags...)
	c := f.addImage("c", "1.0.0", tags...)
	f.group("stack", "1.0.0", a, b, c)
	f.images.OnTagChanged(nil)

	var wg sync.WaitGroup

	for index, id := range []int64{a, b, c, a, b, c} {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := f.images.UpdateTag(f.ctx, id, tags[index%len(tags)], model.TagHistorySourceManual); err != nil {
				t.Errorf("UpdateTag: %v", err)
			}
		}()
	}

	wg.Wait()

	final := f.image(a).Tag

	f.tags(b, final)
	f.tags(c, final)
}

func TestGroupCreateMovesDriftedMembersAndAppliesPin(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.1.0", "1.0.0", "1.1.0", "1.2.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0", "1.2.0")

	if err := f.images.SetPinned(f.ctx, b, true); err != nil {
		t.Fatal(err)
	}

	f.changed = nil

	id := f.group("stack", "1.1.0", a, b)

	f.tags(b, "1.1.0")

	if len(f.changed) != 1 || !slices.Equal(f.changed[0], []string{"b"}) {
		t.Fatalf("tag change notifications = %v, want only the moved image", f.changed)
	}

	group, err := f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if !group.Pinned || !f.image(a).Pinned {
		t.Fatalf("a pinned member must pin the group: %+v", group)
	}

	if group.UpdateAvailable {
		t.Fatalf("a pinned group must not be flagged: %+v", group)
	}
}

func TestSetPinnedAppliesToEveryMember(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	id := f.group("stack", "1.0.0", a, b)

	if err := f.images.SetPinned(f.ctx, a, true); err != nil {
		t.Fatal(err)
	}

	if !f.image(b).Pinned || f.image(a).UpdateAvailable || f.image(b).UpdateAvailable {
		t.Fatalf("pin did not reach every member: %+v %+v", f.image(a), f.image(b))
	}

	if err := f.images.SetPinned(f.ctx, b, false); err != nil {
		t.Fatal(err)
	}

	group, err := f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if group.Pinned || !group.UpdateAvailable || group.UpdateAvailableTag == nil || *group.UpdateAvailableTag != "1.1.0" {
		t.Fatalf("unpin did not restore the group's update state: %+v", group)
	}
}

func TestGroupValidation(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0")
	c := f.addImage("c", "1.0.0", "1.0.0")
	d := f.addImage("d", "2.0.0", "2.0.0")

	cases := []struct {
		name  string
		input ImageGroupInput
		want  error
	}{
		{"blank name", ImageGroupInput{Name: " ", ImageIDs: []int64{a, b}, Tag: "1.0.0"}, ErrInvalidImageGroup},
		{"one image", ImageGroupInput{Name: "x", ImageIDs: []int64{a}, Tag: "1.0.0"}, ErrInvalidImageGroup},
		{"same image twice", ImageGroupInput{Name: "x", ImageIDs: []int64{a, a}, Tag: "1.0.0"}, ErrInvalidImageGroup},
		{"blank tag", ImageGroupInput{Name: "x", ImageIDs: []int64{a, b}}, ErrInvalidImageGroup},
		{"unknown image", ImageGroupInput{Name: "x", ImageIDs: []int64{a, 9999}, Tag: "1.0.0"}, ErrImageNotFound},
		{"tag not on every image", ImageGroupInput{Name: "x", ImageIDs: []int64{a, d}, Tag: "1.0.0"}, ErrTagUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.images.CreateGroup(f.ctx, tc.input); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	groups, err := f.images.CountGroups(f.ctx, model.ImageGroupListFilters{})

	if err != nil || groups != 0 {
		t.Fatalf("failed creates left %d groups behind: %v", groups, err)
	}

	f.group("stack", "1.0.0", a, b)

	if _, err := f.images.CreateGroup(f.ctx, ImageGroupInput{Name: "stack", ImageIDs: []int64{c, d}, Tag: "1.0.0"}); !errors.Is(err, ErrTagUnavailable) {
		t.Fatalf("error = %v, want ErrTagUnavailable", err)
	}

	e := f.addImage("e", "1.0.0", "1.0.0")

	if _, err := f.images.CreateGroup(f.ctx, ImageGroupInput{Name: "stack", ImageIDs: []int64{c, e}, Tag: "1.0.0"}); !errors.Is(err, ErrImageGroupAlreadyExists) {
		t.Fatalf("error = %v, want ErrImageGroupAlreadyExists", err)
	}

	if f.image(c).GroupID != nil {
		t.Fatal("a failed create must not leave membership behind")
	}

	if _, err := f.images.CreateGroup(f.ctx, ImageGroupInput{Name: "other", ImageIDs: []int64{a, c}, Tag: "1.0.0"}); !errors.Is(err, ErrImageInAnotherGroup) {
		t.Fatalf("error = %v, want ErrImageInAnotherGroup", err)
	}
}

func TestUpdateGroupMembership(t *testing.T) {
	f := newGroupFixture(t)

	tags := []string{"1.0.0", "1.1.0"}
	a := f.addImage("a", "1.0.0", tags...)
	b := f.addImage("b", "1.0.0", tags...)
	c := f.addImage("c", "1.1.0", tags...)
	id := f.group("stack", "1.0.0", a, b)

	input := ImageGroupInput{Name: "renamed", ImageIDs: []int64{a, c}, Tag: "1.0.0"}

	if err := f.images.UpdateGroup(f.ctx, id, input); err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}

	f.tags(c, "1.0.0")

	if image := f.image(b); image.GroupID != nil {
		t.Fatalf("removed image still has a group: %+v", image)
	}

	f.tags(b, "1.0.0")

	if err := f.images.UpdateTag(f.ctx, b, "1.1.0", model.TagHistorySourceManual); err != nil {
		t.Fatal(err)
	}

	f.tags(a, "1.0.0")

	group, err := f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if group.Name != "renamed" || !slices.Equal(imageNames(group.Images), []string{"a", "c"}) {
		t.Fatalf("group = %+v", group)
	}

	if f.image(a).GroupName == nil || *f.image(a).GroupName != "renamed" {
		t.Fatalf("image group name = %v", f.image(a).GroupName)
	}
}

func TestDeletingImagesAndGroups(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	id := f.group("stack", "1.0.0", a, b)

	if _, err := f.images.Delete(f.ctx, a); err != nil {
		t.Fatal(err)
	}

	if _, err := f.images.GetGroup(f.ctx, id); err != nil {
		t.Fatalf("group with a member left must survive: %v", err)
	}

	if _, err := f.images.Delete(f.ctx, b); err != nil {
		t.Fatal(err)
	}

	if _, err := f.images.GetGroup(f.ctx, id); !errors.Is(err, ErrImageGroupNotFound) {
		t.Fatalf("error = %v, want the empty group gone", err)
	}

	c := f.addImage("c", "1.0.0", "1.0.0", "1.1.0")
	d := f.addImage("d", "1.0.0", "1.0.0", "1.1.0")
	second := f.group("second", "1.0.0", c, d)

	if err := f.images.DeleteGroup(f.ctx, second); err != nil {
		t.Fatal(err)
	}

	for _, imageID := range []int64{c, d} {
		image := f.image(imageID)

		if image.GroupID != nil || image.Tag != "1.0.0" || !image.UpdateAvailable {
			t.Fatalf("deleting a group must leave images intact: %+v", image)
		}
	}
}

func TestGroupUpdateStateAcrossARelease(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	c := f.addImage("c", "1.0.0", "1.0.0")
	id := f.group("stack", "1.0.0", a, b, c)

	group, err := f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if !group.UpdateAvailable || group.UpdateAvailableTag != nil {
		t.Fatalf("expected a blocked group: %+v", group)
	}

	if group.PartialTag == nil || group.PartialTag.Tag != "1.1.0" || !slices.Equal(group.PartialTag.Missing, []string{"c"}) {
		t.Fatalf("partial tag = %+v, want 1.1.0 missing from c", group.PartialTag)
	}

	tags, err := f.images.GetTags(f.ctx, a)

	if err != nil {
		t.Fatal(err)
	}

	if len(tags) != 1 || tags[0].Tag != "1.0.0" {
		t.Fatalf("selectable tags = %+v, want only the common tag", tags)
	}

	f.refresh(c, "1.0.0", "1.1.0")

	group, err = f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if group.UpdateAvailableTag == nil || *group.UpdateAvailableTag != "1.1.0" || group.PartialTag != nil {
		t.Fatalf("release reached every member, want a plain update: %+v", group)
	}

	f.refresh(a, "1.0.0", "1.1.0", "1.2.0")

	group, err = f.images.GetGroup(f.ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	if group.UpdateAvailableTag == nil || *group.UpdateAvailableTag != "1.1.0" {
		t.Fatalf("want the newest common tag offered: %+v", group)
	}

	if group.PartialTag == nil || group.PartialTag.Tag != "1.2.0" || !slices.Equal(group.PartialTag.Missing, []string{"b", "c"}) {
		t.Fatalf("partial tag = %+v, want 1.2.0 missing from b and c", group.PartialTag)
	}
}

func TestGroupSelectableTagsMergeNewFlags(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0")
	b := f.addImage("b", "1.0.0", "1.0.0")
	f.group("stack", "1.0.0", a, b)

	f.refresh(b, "1.0.0", "1.1.0")
	f.refresh(a, "1.0.0", "1.1.0")

	if _, err := f.store.MarkImageTagsAsSeen(f.ctx, a); err != nil {
		t.Fatal(err)
	}

	tags, err := f.images.GetTags(f.ctx, a)

	if err != nil {
		t.Fatal(err)
	}

	index := slices.IndexFunc(tags, func(tag model.ImageTag) bool { return tag.Tag == "1.1.0" })

	if index == -1 || !tags[index].New {
		t.Fatalf("tags = %+v, want 1.1.0 still new because b has not seen it", tags)
	}

	if _, err := f.images.MarkTagsAsSeen(f.ctx, a); err != nil {
		t.Fatal(err)
	}

	tags, err = f.images.GetTags(f.ctx, b)

	if err != nil {
		t.Fatal(err)
	}

	for _, tag := range tags {
		if tag.New {
			t.Fatalf("mark-seen must cover every member: %+v", tags)
		}
	}
}

func TestCommonTags(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0", "1.2.0")
	b := f.addImage("b", "1.1.0", "1.0.0", "1.1.0")

	tags, err := f.images.CommonTags(f.ctx, []int64{a, b})

	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(tags))

	for _, tag := range tags {
		names = append(names, tag.Tag)
	}

	if !slices.Equal(names, []string{"1.1.0", "1.0.0"}) {
		t.Fatalf("common tags = %v", names)
	}
}

func TestStatusCountsAndPerImageUpdateCounts(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0", "1.1.0")
	b := f.addImage("b", "1.0.0", "1.0.0", "1.1.0")
	f.addImage("c", "1.0.0", "1.0.0", "1.1.0")
	f.addImage("d", "1.1.0", "1.0.0", "1.1.0")
	f.group("stack", "1.0.0", a, b)

	counts, err := f.images.StatusCounts(f.ctx)

	if err != nil {
		t.Fatal(err)
	}

	if counts.Total != 4 || counts.Registries != 1 {
		t.Fatalf("counts = %+v, want 4 images on 1 registry", counts)
	}

	all, err := f.images.Count(f.ctx, model.ImageListFilters{Status: model.ImageStatusUpdateAvailable})

	if err != nil || all != 3 {
		t.Fatalf("image list count = %d (%v), want it to stay per image", all, err)
	}

	ungrouped, err := f.images.Count(f.ctx, model.ImageListFilters{Status: model.ImageStatusUpdateAvailable, Ungrouped: true})

	if err != nil || ungrouped != 1 {
		t.Fatalf("ungrouped count = %d (%v), want 1", ungrouped, err)
	}

	flagged, err := f.images.CountGroups(f.ctx, model.ImageGroupListFilters{UpdateAvailable: true})

	if err != nil || flagged != 1 {
		t.Fatalf("flagged groups = %d (%v), want 1", flagged, err)
	}
}

func TestListGroupsSearchesMembers(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("alpha", "1.0.0", "1.0.0")
	b := f.addImage("beta", "1.0.0", "1.0.0")
	f.group("stack", "1.0.0", a, b)

	for search, want := range map[string]int64{"sta": 1, "beta": 1, "stack/alpha": 1, "nothing": 0} {
		count, err := f.images.CountGroups(f.ctx, model.ImageGroupListFilters{Search: search})

		if err != nil || count != want {
			t.Fatalf("search %q = %d (%v), want %d", search, count, err, want)
		}
	}
}

func TestImageListFindsGroupsAndMembers(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("alpha", "1.0.0", "1.0.0")
	b := f.addImage("beta", "1.0.0", "1.0.0")
	f.addImage("gamma", "1.0.0", "1.0.0")
	id := f.group("Shared stack", "1.0.0", a, b)

	cases := []struct {
		name    string
		filters model.ImageListFilters
		want    []string
	}{
		{"by group", model.ImageListFilters{GroupID: id}, []string{"beta", "alpha"}},
		{"search by group name", model.ImageListFilters{Search: "shared"}, []string{"beta", "alpha"}},
		{"search still finds ungrouped", model.ImageListFilters{Search: "gamma"}, []string{"gamma"}},
		{"ungrouped only", model.ImageListFilters{Ungrouped: true}, []string{"gamma"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.filters.Limit = 10

			images, err := f.images.List(f.ctx, tc.filters)

			if err != nil {
				t.Fatal(err)
			}

			if got := imageNames(images); !slices.Equal(got, tc.want) {
				t.Fatalf("images = %v, want %v", got, tc.want)
			}

			count, err := f.images.Count(f.ctx, tc.filters)

			if err != nil || count != int64(len(tc.want)) {
				t.Fatalf("count = %d (%v), want %d", count, err, len(tc.want))
			}
		})
	}
}

func TestListGroupsPinnedFilter(t *testing.T) {
	f := newGroupFixture(t)

	a := f.addImage("a", "1.0.0", "1.0.0")
	b := f.addImage("b", "1.0.0", "1.0.0")
	c := f.addImage("c", "1.0.0", "1.0.0")
	d := f.addImage("d", "1.0.0", "1.0.0")
	f.group("first", "1.0.0", a, b)
	f.group("second", "1.0.0", c, d)

	if err := f.images.SetPinned(f.ctx, c, true); err != nil {
		t.Fatal(err)
	}

	count, err := f.images.CountGroups(f.ctx, model.ImageGroupListFilters{Pinned: true})

	if err != nil || count != 1 {
		t.Fatalf("pinned groups = %d (%v), want 1", count, err)
	}
}

func TestGroupRefreshOrderPutsRequestedImageLast(t *testing.T) {
	members := []model.Image{{ID: 1}, {ID: 2}, {ID: 3}}

	if got := groupRefreshOrder(members, 2); !slices.Equal(got, []int64{1, 3, 2}) {
		t.Fatalf("order = %v", got)
	}
}
