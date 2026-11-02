package service

import (
	"context"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type proxyUsageStore interface {
	RecordProxyUsage(ctx context.Context, images []model.ImageUsage, tokens []model.ProxyTokenUsage) error
}

type ProxyUsage struct {
	store proxyUsageStore
	now   func() time.Time

	mu     sync.Mutex
	images map[int64]model.ImageUsage
	tokens map[int64]time.Time
}

func NewProxyUsage(store proxyUsageStore) *ProxyUsage {
	return &ProxyUsage{
		store:  store,
		now:    time.Now,
		images: make(map[int64]model.ImageUsage),
		tokens: make(map[int64]time.Time),
	}
}

func (u *ProxyUsage) RecordImagePull(imageID int64, tag string) {
	now := u.now().UTC()

	u.mu.Lock()
	defer u.mu.Unlock()

	u.images[imageID] = model.ImageUsage{ImageID: imageID, PulledAt: now, PulledTag: tag}
}

func (u *ProxyUsage) RecordTokenUse(tokenID int64) {
	now := u.now().UTC()

	u.mu.Lock()
	defer u.mu.Unlock()

	u.tokens[tokenID] = now
}

func (u *ProxyUsage) Flush(ctx context.Context) error {
	u.mu.Lock()
	images, tokens := u.images, u.tokens
	u.images = make(map[int64]model.ImageUsage)
	u.tokens = make(map[int64]time.Time)
	u.mu.Unlock()

	if len(images) == 0 && len(tokens) == 0 {
		return nil
	}

	imageUsage := make([]model.ImageUsage, 0, len(images))

	for _, usage := range images {
		imageUsage = append(imageUsage, usage)
	}

	tokenUsage := make([]model.ProxyTokenUsage, 0, len(tokens))

	for id, usedAt := range tokens {
		tokenUsage = append(tokenUsage, model.ProxyTokenUsage{TokenID: id, UsedAt: usedAt})
	}

	err := u.store.RecordProxyUsage(ctx, imageUsage, tokenUsage)

	if err != nil {
		u.restore(images, tokens)
	}

	return err
}

func (u *ProxyUsage) restore(images map[int64]model.ImageUsage, tokens map[int64]time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for id, older := range images {
		if _, ok := u.images[id]; !ok {
			u.images[id] = older
		}
	}

	for id, usedAt := range tokens {
		if _, ok := u.tokens[id]; !ok {
			u.tokens[id] = usedAt
		}
	}
}

func (u *ProxyUsage) ApplyToImages(images []model.Image) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for i := range images {
		if usage, ok := u.images[images[i].ID]; ok {
			images[i].LastPulledAt = &usage.PulledAt
			images[i].LastPulledTag = &usage.PulledTag
		}
	}
}

func (u *ProxyUsage) ApplyToTokens(tokens []model.ProxyToken) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for i := range tokens {
		if usedAt, ok := u.tokens[tokens[i].ID]; ok {
			tokens[i].LastUsedAt = &usedAt
		}
	}
}
