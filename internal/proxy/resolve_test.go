package proxy

import (
	"context"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
)

func TestResolveCacheInvalidateDropsEntry(t *testing.T) {
	cache := newResolveCache()
	tag := "1.0"

	lookup := func(context.Context, string) (*model.Image, error) {
		return &model.Image{Name: "app", Tag: tag}, nil
	}

	if _, err := cache.resolve(context.Background(), "app", lookup); err != nil {
		t.Fatal(err)
	}

	tag = "2.0"

	cached, err := cache.resolve(context.Background(), "app", lookup)

	if err != nil {
		t.Fatal(err)
	}

	if cached.Tag != "1.0" {
		t.Fatalf("cached tag = %q, want 1.0", cached.Tag)
	}

	cache.invalidate("app")

	fresh, err := cache.resolve(context.Background(), "app", lookup)

	if err != nil {
		t.Fatal(err)
	}

	if fresh.Tag != "2.0" {
		t.Fatalf("tag after invalidate = %q, want 2.0", fresh.Tag)
	}
}

func TestResolveCacheSkipsLookupStartedBeforeInvalidate(t *testing.T) {
	cache := newResolveCache()

	lookup := func(context.Context, string) (*model.Image, error) {
		cache.invalidate("app")

		return &model.Image{Name: "app", Tag: "stale"}, nil
	}

	if _, err := cache.resolve(context.Background(), "app", lookup); err != nil {
		t.Fatal(err)
	}

	if _, ok := cache.entries.Get("app"); ok {
		t.Fatal("a lookup that raced an invalidate must not be cached")
	}
}
