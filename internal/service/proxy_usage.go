package service

import (
	"context"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type proxyUsageStore interface {
	RecordProxyUsage(ctx context.Context, images []model.ImageUsage, digests []model.ImageDigest, tokens []model.ProxyTokenUsage) error
}

type ProxyUsage struct {
	store proxyUsageStore
	now   func() time.Time

	mu      sync.Mutex
	images  map[int64]model.ImageUsage
	digests map[int64]model.ImageDigest
	tokens  map[int64]time.Time
}

func NewProxyUsage(store proxyUsageStore) *ProxyUsage {
	return &ProxyUsage{
		store:   store,
		now:     time.Now,
		images:  make(map[int64]model.ImageUsage),
		digests: make(map[int64]model.ImageDigest),
		tokens:  make(map[int64]time.Time),
	}
}

func (u *ProxyUsage) RecordImagePull(imageID int64, tag string) {
	now := u.now().UTC()

	u.mu.Lock()
	defer u.mu.Unlock()

	u.images[imageID] = model.ImageUsage{ImageID: imageID, PulledAt: now, PulledTag: tag}
}

func (u *ProxyUsage) RecordTagDigest(imageID int64, tag, digest string) {
	now := u.now().UTC()

	u.mu.Lock()
	defer u.mu.Unlock()

	if seen, ok := u.digests[imageID]; ok && seen.Tag == tag && seen.Digest == digest {
		return
	}

	u.digests[imageID] = model.ImageDigest{ImageID: imageID, Tag: tag, Digest: digest, SeenAt: now}
}

func (u *ProxyUsage) RecordTokenUse(tokenID int64) {
	now := u.now().UTC()

	u.mu.Lock()
	defer u.mu.Unlock()

	u.tokens[tokenID] = now
}

func (u *ProxyUsage) Flush(ctx context.Context) error {
	u.mu.Lock()
	images, digests, tokens := u.images, u.digests, u.tokens
	u.images = make(map[int64]model.ImageUsage)
	u.digests = make(map[int64]model.ImageDigest)
	u.tokens = make(map[int64]time.Time)
	u.mu.Unlock()

	if len(images) == 0 && len(digests) == 0 && len(tokens) == 0 {
		return nil
	}

	imageUsage := make([]model.ImageUsage, 0, len(images))

	for _, usage := range images {
		imageUsage = append(imageUsage, usage)
	}

	seenDigests := make([]model.ImageDigest, 0, len(digests))

	for _, seen := range digests {
		seenDigests = append(seenDigests, seen)
	}

	tokenUsage := make([]model.ProxyTokenUsage, 0, len(tokens))

	for id, usedAt := range tokens {
		tokenUsage = append(tokenUsage, model.ProxyTokenUsage{TokenID: id, UsedAt: usedAt})
	}

	err := u.store.RecordProxyUsage(ctx, imageUsage, seenDigests, tokenUsage)

	if err != nil {
		u.restore(images, digests, tokens)
	}

	return err
}

func (u *ProxyUsage) restore(images map[int64]model.ImageUsage, digests map[int64]model.ImageDigest, tokens map[int64]time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for id, older := range images {
		if _, ok := u.images[id]; !ok {
			u.images[id] = older
		}
	}

	for id, older := range digests {
		if _, ok := u.digests[id]; !ok {
			u.digests[id] = older
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

		u.applyTagDigest(&images[i])
	}
}

func (u *ProxyUsage) ApplyTagDigest(image *model.Image) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.applyTagDigest(image)
}

func (u *ProxyUsage) applyTagDigest(image *model.Image) {
	seen, ok := u.digests[image.ID]

	if !ok || seen.Tag != image.Tag {
		return
	}

	if stored, _ := image.TagDigest(); stored != nil && *stored == seen.Digest {
		return
	}

	image.CurrentDigest = &seen.Digest
	image.CurrentDigestTag = &seen.Tag
	image.CurrentDigestSince = &seen.SeenAt
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
