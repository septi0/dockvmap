package service

import (
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func TestImageListSorting(t *testing.T) {
	f := newGroupFixture(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	bravo := f.addImage("bravo", "1.0.0", "1.0.0", "1.1.0")
	alpha := f.addImage("alpha", "2.0.0", "2.0.0")
	charlie := f.addImage("charlie", "1.0.0", "1.0.0")
	delta := f.addImage("delta", "1.0.0", "1.0.0")
	echo := f.addImage("echo", "1.0.0", "1.0.0")

	if err := f.images.SetPinned(f.ctx, charlie, true); err != nil {
		t.Fatalf("pinning: %v", err)
	}

	checkErr := "unexpected status 401 Unauthorized"

	for id, check := range map[int64]struct {
		hoursAgo int
		err      *string
	}{alpha: {5, &checkErr}, bravo: {1, nil}, charlie: {4, nil}, delta: {2, nil}, echo: {3, nil}} {
		if _, err := f.store.UpdateImageCheck(f.ctx, nil, id, check.err, base.Add(-time.Duration(check.hoursAgo)*time.Hour), f.image(id).TagSetHash); err != nil {
			t.Fatalf("setting check: %v", err)
		}
	}

	usage, clock := newTestProxyUsage(f.store)
	usage.RecordImageRequest(bravo, "1.0.0", true)
	clock.advance(time.Hour)
	usage.RecordImageRequest(delta, "1.0.0", true)

	if err := usage.Flush(f.ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	names := func(filters model.ImageListFilters) []string {
		t.Helper()

		if filters.Limit == 0 {
			filters.Limit = 50
		}

		images, err := f.store.ListImages(f.ctx, filters)

		if err != nil {
			t.Fatalf("listing: %v", err)
		}

		out := make([]string, 0, len(images))

		for _, image := range images {
			out = append(out, image.Name)
		}

		return out
	}

	sorted := func(field string, direction model.SortDirection) model.ImageListFilters {
		return model.ImageListFilters{Sort: model.Sort{Field: field, Direction: direction}}
	}

	tests := []struct {
		name    string
		filters model.ImageListFilters
		want    []string
	}{
		{"no sort keeps newest added first", model.ImageListFilters{}, []string{"echo", "delta", "charlie", "alpha", "bravo"}},
		{"unknown field keeps the default order", sorted("password_hash", model.SortAscending), []string{"echo", "delta", "charlie", "alpha", "bravo"}},
		{"name ascending", sorted(model.ImageSortName, model.SortAscending), []string{"alpha", "bravo", "charlie", "delta", "echo"}},
		{"name descending", sorted(model.ImageSortName, model.SortDescending), []string{"echo", "delta", "charlie", "bravo", "alpha"}},
		{"status puts what needs attention first", sorted(model.ImageSortStatus, model.SortAscending), []string{"bravo", "alpha", "charlie", "delta", "echo"}},
		{"status descending breaks ties by name", sorted(model.ImageSortStatus, model.SortDescending), []string{"delta", "echo", "charlie", "alpha", "bravo"}},
		{"last checked newest first", sorted(model.ImageSortLastChecked, model.SortDescending), []string{"bravo", "delta", "echo", "charlie", "alpha"}},
		{"last checked oldest first", sorted(model.ImageSortLastChecked, model.SortAscending), []string{"alpha", "charlie", "echo", "delta", "bravo"}},
		{"last pulled newest first, never pulled last", sorted(model.ImageSortLastPulled, model.SortDescending), []string{"delta", "bravo", "alpha", "charlie", "echo"}},
		{"last pulled oldest first, never pulled still last", sorted(model.ImageSortLastPulled, model.SortAscending), []string{"bravo", "delta", "alpha", "charlie", "echo"}},
		{"sort pages without gaps", model.ImageListFilters{Pagination: model.Pagination{Offset: 2, Limit: 2}, Sort: model.Sort{Field: model.ImageSortName, Direction: model.SortAscending}}, []string{"charlie", "delta"}},
		{"sort combines with filters", model.ImageListFilters{Search: "ha", Sort: model.Sort{Field: model.ImageSortName, Direction: model.SortDescending}}, []string{"charlie", "alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(tt.filters)

			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
