package proxy

import (
	"context"
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
	entries *expirable.LRU[string, model.Image]
	group   singleflight.Group
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

		img, err := lookup(ctx, name)

		if err != nil {
			return (*model.Image)(nil), err
		}

		if img != nil {
			c.entries.Add(name, *img)
		}

		return img, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*model.Image), nil
}
