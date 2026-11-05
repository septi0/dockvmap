package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

const digestLookupTimeout = 10 * time.Second

func (i *Images) refreshDigest(ctx context.Context, image *model.Image) {
	lookupCtx, cancel := context.WithTimeout(ctx, digestLookupTimeout)
	digest, err := i.digests.ManifestDigest(lookupCtx, image.Registry, image.Repository, image.Tag)
	cancel()

	if err != nil {
		i.failures.Record(ctx, FailureSourceDigest, image.Name, err)

		return
	}

	var stored *string

	if digest != "" {
		stored = &digest
	}

	if err := i.store.UpdateImageDigest(ctx, image.ID, image.Tag, stored, time.Now().UTC()); err != nil {
		slog.Error("recording image digest failed", "image", image.Name, "error", err)
	}
}

func (i *Images) RecordServedDigest(imageID int64, tag, digest string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), digestLookupTimeout)
		defer cancel()

		if err := i.store.UpdateImageDigest(ctx, imageID, tag, &digest, time.Now().UTC()); err != nil {
			slog.Error("recording a served digest failed", "image", imageID, "error", err)
		}
	}()
}

func (i *Images) refreshDigestsInBackground(names []string) {
	go func() {
		for _, name := range names {
			i.refreshDigestByName(name)
		}
	}()
}

func (i *Images) refreshDigestByName(name string) {
	image, err := i.store.GetImage(i.bgCtx, name)

	if err != nil {
		slog.Error("loading image for digest lookup failed", "image", name, "error", err)

		return
	}

	if image != nil {
		i.refreshDigest(i.bgCtx, image)
	}
}
