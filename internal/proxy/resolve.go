package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"

	"github.com/septi0/dockvmap/internal/model"
)

const (
	resolveCacheMaxEntries = 1024
	resolveCacheTTL        = 20 * time.Second
)

type resolveCache struct {
	entries    *expirable.LRU[string, model.Image]
	group      singleflight.Group
	mu         sync.Mutex
	generation uint64
}

func newResolveCache() *resolveCache {
	return &resolveCache{
		entries: expirable.NewLRU[string, model.Image](resolveCacheMaxEntries, nil, resolveCacheTTL),
	}
}

func (c *resolveCache) resolve(ctx context.Context, name string, lookup func(context.Context, string) (*model.Image, error)) (*model.Image, error) {
	if img, ok := c.entries.Get(name); ok {
		copied := img
		return &copied, nil
	}

	result, err, _ := c.group.Do(name, func() (any, error) {
		if img, ok := c.entries.Get(name); ok {
			copied := img
			return &copied, nil
		}

		generation := c.currentGeneration()

		img, err := lookup(ctx, name)

		if err != nil {
			return (*model.Image)(nil), err
		}

		if img != nil {
			c.store(name, *img, generation)
		}

		return img, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*model.Image), nil
}

func (c *resolveCache) currentGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.generation
}

func (c *resolveCache) store(name string, img model.Image, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if generation == c.generation {
		c.entries.Add(name, img)
	}
}

func (c *resolveCache) invalidate(names ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.generation++

	for _, name := range names {
		c.entries.Remove(name)
		c.group.Forget(name)
	}
}
