package service

import (
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

// mt builds an ImageTag for updateAvailableFor tests. Pass segs as zero or more
// version axes; none yields a nil VersionSegments (date/alphabetical families).
func mt(tag string, family int64, order int, prerelease, hasOrder bool, segs ...[]int64) model.ImageTag {
	return model.ImageTag{
		Tag:             tag,
		FamilyID:        family,
		TagOrder:        order,
		Prerelease:      prerelease,
		FamilyHasOrder:  hasOrder,
		VersionSegments: segs,
	}
}

func TestUpdateAvailableFor(t *testing.T) {
	cases := []struct {
		name      string
		tags      []model.ImageTag
		current   string
		wantAvail bool
		wantTag   string
	}{
		{
			name:    "current tag not present in the set",
			tags:    []model.ImageTag{mt("1.3", 1, 0, false, true, []int64{1, 3})},
			current: "absent",
		},
		{
			name: "family has no order (hash-only) never recommends",
			tags: []model.ImageTag{
				mt("cur", 1, 1, false, false),
				mt("new", 1, 0, false, false),
			},
			current: "cur",
		},
		{
			name: "current is already the newest in its family",
			tags: []model.ImageTag{
				mt("1.3", 1, 0, false, true, []int64{1, 3}),
				mt("1.2", 1, 1, false, true, []int64{1, 2}),
			},
			current: "1.3",
		},
		{
			name: "straightforward upgrade available",
			tags: []model.ImageTag{
				mt("1.3", 1, 0, false, true, []int64{1, 3}),
				mt("1.2", 1, 1, false, true, []int64{1, 2}),
			},
			current:   "1.2",
			wantAvail: true,
			wantTag:   "1.3",
		},
		{
			name: "picks the newest candidate, not the nearest",
			tags: []model.ImageTag{
				mt("1.4", 1, 0, false, true, []int64{1, 4}),
				mt("1.3", 1, 1, false, true, []int64{1, 3}),
				mt("1.2", 1, 2, false, true, []int64{1, 2}),
			},
			current:   "1.2",
			wantAvail: true,
			wantTag:   "1.4",
		},
		{
			name: "candidate in a different family is ignored",
			tags: []model.ImageTag{
				mt("cur", 1, 1, false, true, []int64{1, 2}),
				mt("2.0", 2, 0, false, true, []int64{2, 0}),
			},
			current: "cur",
		},
		{
			name: "pinned 1.2 already tracks 1.2.6 (prefix relation, suppressed)",
			tags: []model.ImageTag{
				mt("1.2.6", 1, 0, false, true, []int64{1, 2, 6}),
				mt("1.2", 1, 1, false, true, []int64{1, 2}),
			},
			current: "1.2",
		},
		{
			name: "pinned 1.2, candidate 1.3.0 is a real minor gap despite extra axis",
			tags: []model.ImageTag{
				mt("1.3.0", 1, 0, false, true, []int64{1, 3, 0}),
				mt("1.2", 1, 1, false, true, []int64{1, 2}),
			},
			current:   "1.2",
			wantAvail: true,
			wantTag:   "1.3.0",
		},
		{
			name: "pinned 10.11 does not flag a dated build 10.11.11.<date>-<time>",
			tags: []model.ImageTag{
				mt("10.11.11.20260606-153911", 1, 0, false, true, []int64{10, 11, 11}, []int64{20260606}, []int64{153911}),
				mt("10.11", 1, 1, false, true, []int64{10, 11}),
			},
			current: "10.11",
		},
		{
			name: "base-image-only bump 17.9-alpine-3.23 -> 17.9-alpine-3.24 is still reported",
			tags: []model.ImageTag{
				mt("17.9-alpine-3.24", 1, 0, false, true, []int64{17, 9}, []int64{3, 24}),
				mt("17.9-alpine-3.23", 1, 1, false, true, []int64{17, 9}, []int64{3, 23}),
			},
			current:   "17.9-alpine-3.23",
			wantAvail: true,
			wantTag:   "17.9-alpine-3.24",
		},
		{
			name: "prefix on the base axis: 17.9-alpine-3 -> 17.9-alpine-3.23 is suppressed",
			tags: []model.ImageTag{
				mt("17.9-alpine-3.23", 1, 0, false, true, []int64{17, 9}, []int64{3, 23}),
				mt("17.9-alpine-3", 1, 1, false, true, []int64{17, 9}, []int64{3}),
			},
			current: "17.9-alpine-3",
		},
		{
			name: "pinned 17 does not flag 17.10 (impl suppresses; PROJECT.md:172 wording says it should)",
			tags: []model.ImageTag{
				mt("17.10", 1, 0, false, true, []int64{17, 10}),
				mt("17", 1, 1, false, true, []int64{17}),
			},
			current: "17",
		},
		{
			name: "prerelease candidate is not offered to a stable pin",
			tags: []model.ImageTag{
				mt("2.0.0-rc1", 1, 0, true, true, []int64{2, 0, 0}),
				mt("1.9", 1, 1, false, true, []int64{1, 9}),
			},
			current: "1.9",
		},
		{
			name: "prerelease pin may move to a newer prerelease",
			tags: []model.ImageTag{
				mt("2.0.0-rc1", 1, 0, true, true, []int64{2, 0, 0}),
				mt("1.9", 1, 1, true, true, []int64{1, 9}),
			},
			current:   "1.9",
			wantAvail: true,
			wantTag:   "2.0.0-rc1",
		},
		{
			name: "date-led family (empty version segments) still reports an upgrade",
			tags: []model.ImageTag{
				mt("2024-02-01", 1, 0, false, true),
				mt("2024-01-01", 1, 1, false, true),
			},
			current:   "2024-01-01",
			wantAvail: true,
			wantTag:   "2024-02-01",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotAvail, gotTag := updateAvailableFor(tc.tags, tc.current)

			if gotAvail != tc.wantAvail || gotTag != tc.wantTag {
				t.Errorf("updateAvailableFor(%q) = (%v, %q); want (%v, %q)",
					tc.current, gotAvail, gotTag, tc.wantAvail, tc.wantTag)
			}
		})
	}
}

func TestEffectiveUpdateAvailable(t *testing.T) {
	tags := []model.ImageTag{
		mt("1.3.0", 1, 0, false, true, []int64{1, 3, 0}),
		mt("1.2.0", 1, 1, false, true, []int64{1, 2, 0}),
	}

	if avail, tag := effectiveUpdateAvailable(false, tags, "1.2.0"); !avail || tag != "1.3.0" {
		t.Errorf("unpinned = (%v, %q); want (true, %q)", avail, tag, "1.3.0")
	}

	if avail, tag := effectiveUpdateAvailable(true, tags, "1.2.0"); avail || tag != "" {
		t.Errorf("pinned = (%v, %q); want (false, \"\")", avail, tag)
	}
}

func TestVersionContains(t *testing.T) {
	cases := []struct {
		name  string
		outer [][]int64
		inner [][]int64
		want  bool
	}{
		{"empty outer never contains", nil, [][]int64{{1}}, false},
		{"equal is not more specific", [][]int64{{1, 2}}, [][]int64{{1, 2}}, false},
		{"longer trailing axis", [][]int64{{1, 2}}, [][]int64{{1, 2, 6}}, true},
		{"not a numeric prefix", [][]int64{{1, 2}}, [][]int64{{1, 3, 0}}, false},
		{"extra trailing axes count as more specific", [][]int64{{10, 11}}, [][]int64{{10, 11, 11}, {20260606}}, true},
		{"divergence on a later axis", [][]int64{{17, 9}, {3, 23}}, [][]int64{{17, 9}, {3, 24}}, false},
		{"prefix on the second axis", [][]int64{{17, 9}, {3}}, [][]int64{{17, 9}, {3, 23}}, true},
		{"outer longer than inner", [][]int64{{1, 2}, {3}}, [][]int64{{1, 2}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionContains(tc.outer, tc.inner); got != tc.want {
				t.Errorf("versionContains(%v, %v) = %v; want %v", tc.outer, tc.inner, got, tc.want)
			}
		})
	}
}

func TestTagSetHash(t *testing.T) {
	if tagSetHash([]string{"a", "b", "c"}) != tagSetHash([]string{"c", "a", "b"}) {
		t.Error("hash must not depend on input order")
	}

	if tagSetHash([]string{"a", "b"}) == tagSetHash([]string{"a", "b", "c"}) {
		t.Error("adding a tag must change the hash")
	}

	if tagSetHash([]string{"ab", "c"}) == tagSetHash([]string{"a", "bc"}) {
		t.Error("tag boundaries must be significant")
	}

	if tagSetHash(nil) != tagSetHash([]string{}) {
		t.Error("nil and empty slice must hash alike")
	}

	if tagSetHash(nil) == tagSetHash([]string{"a"}) {
		t.Error("empty and non-empty sets must differ")
	}

	if tagSetHash(nil) == "" {
		t.Error("hash must be a non-empty digest")
	}
}

func TestNumberPrefix(t *testing.T) {
	cases := []struct {
		name   string
		prefix []int64
		full   []int64
		want   bool
	}{
		{"both empty", nil, nil, true},
		{"prefix longer than empty full", []int64{1}, nil, false},
		{"strict prefix", []int64{1, 2}, []int64{1, 2, 0}, true},
		{"divergent tail", []int64{1, 2}, []int64{1, 3}, false},
		{"prefix longer than full", []int64{1, 2, 3}, []int64{1, 2}, false},
		{"equal", []int64{1, 2}, []int64{1, 2}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := numberPrefix(tc.prefix, tc.full); got != tc.want {
				t.Errorf("numberPrefix(%v, %v) = %v; want %v", tc.prefix, tc.full, got, tc.want)
			}
		})
	}
}
