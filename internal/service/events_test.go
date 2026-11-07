package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

func TestEventListShowsAnAddedUpgradeOnce(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	registryID, err := st.CreateRegistry(ctx, model.Registry{Registry: "registry.example.com"})

	if err != nil {
		t.Fatalf("creating registry: %v", err)
	}

	image := &model.Image{Name: "postgres", RegistryID: registryID, Repository: "library/postgres", Tag: "16.4"}

	if err := st.CreateImage(ctx, image); err != nil {
		t.Fatalf("creating image: %v", err)
	}

	add := func(eventType string, tags ...string) {
		if err := st.AddTagsEvent(ctx, image.ID, eventType, false, model.TagsEventData{Tags: tags}); err != nil {
			t.Fatalf("adding %s: %v", eventType, err)
		}
	}

	add(EventTypeTagAdded, "17.0")
	add(EventTypeUpgradeAvailable, "17.0")
	add(EventTypeTagAdded, "18.0", "17.3")
	add(EventTypeUpgradeAvailable, "18.0")

	events := NewEvents(st)
	list := func(eventType string) []string {
		t.Helper()

		filters := model.ImageEventListFilters{Pagination: model.Pagination{Limit: 10}, Type: eventType}
		found, err := events.List(ctx, filters)

		if err != nil {
			t.Fatalf("listing %q: %v", eventType, err)
		}

		count, err := events.Count(ctx, filters)

		if err != nil || count != int64(len(found)) {
			t.Fatalf("count %q = %d (err %v), want %d like the list", eventType, count, err, len(found))
		}

		rows := make([]string, len(found))

		for i, event := range found {
			rows[i] = event.Type + " " + event.Data.Tags[0]
		}

		return rows
	}

	if got := list(""); len(got) != 3 || got[0] != "UPGRADE_AVAILABLE 18.0" || got[1] != "TAG_ADDED 18.0" || got[2] != "UPGRADE_AVAILABLE 17.0" {
		t.Errorf("all events = %v, want the 18.0 upgrade, the mixed 18.0+17.3 add and the 17.0 upgrade", got)
	}

	if got := list(EventTypeTagAdded); len(got) != 2 {
		t.Errorf("tag added events = %v, want both adds when filtering by type", got)
	}
}
