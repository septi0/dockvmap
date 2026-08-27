package service

import (
	"context"
	"sort"

	"github.com/septi0/dockvmap/internal/model"
)

type EventMode int

const (
	EventNone EventMode = iota
	EventNormal
	EventSilent
)

const (
	EventTypeTagAdded         = "TAG_ADDED"
	EventTypeTagRemoved       = "TAG_REMOVED"
	EventTypeUpgradeAvailable = "UPGRADE_AVAILABLE"
)

type eventStore interface {
	AddTagsEvent(ctx context.Context, imageID int64, eventType string, notify bool, data model.TagsEventData) error
	ListTagsEvents(ctx context.Context, offset, limit int) ([]model.ImageEvent, error)
}

type eventHandler interface {
	HandleEvent(ctx context.Context, mode EventMode, image *model.Image, oldTags []model.ImageTag, newTags []model.ImageTag, updateAvailable bool, updateAvailableTag string) error
}

type Events struct {
	store eventStore
}

func NewEvents(store eventStore) *Events {
	return &Events{store: store}
}

func (e *Events) HandleEvent(ctx context.Context, mode EventMode, image *model.Image, oldTags []model.ImageTag, newTags []model.ImageTag, updateAvailable bool, updateAvailableTag string) error {
	if mode == EventNone {
		return nil
	}

	addedTags, removedTags := diffImageTags(oldTags, newTags)

	if len(addedTags) > 0 {
		data := model.TagsEventData{Tags: tagNames(addedTags)}

		if err := e.store.AddTagsEvent(ctx, image.ID, EventTypeTagAdded, (mode == EventNormal), data); err != nil {
			return err
		}
	}

	if len(removedTags) > 0 {
		data := model.TagsEventData{Tags: tagNames(removedTags)}

		if err := e.store.AddTagsEvent(ctx, image.ID, EventTypeTagRemoved, (mode == EventNormal), data); err != nil {
			return err
		}
	}

	if upgradeBecameAvailable(image, updateAvailable, updateAvailableTag) {
		data := model.TagsEventData{Tags: []string{updateAvailableTag}}

		if err := e.store.AddTagsEvent(ctx, image.ID, EventTypeUpgradeAvailable, (mode == EventNormal), data); err != nil {
			return err
		}
	}

	return nil
}

func upgradeBecameAvailable(image *model.Image, updateAvailable bool, updateAvailableTag string) bool {
	if !updateAvailable || updateAvailableTag == "" {
		return false
	}

	if !image.UpdateAvailable {
		return true
	}

	return image.UpdateAvailableTag == nil || *image.UpdateAvailableTag != updateAvailableTag
}

func (e *Events) List(ctx context.Context, offset, limit int) ([]model.ImageEvent, error) {
	return e.store.ListTagsEvents(ctx, offset, limit)
}

func diffImageTags(oldTags []model.ImageTag, newTags []model.ImageTag) ([]model.ImageTag, []model.ImageTag) {
	addedTags := make([]model.ImageTag, 0, len(newTags))
	removedTags := make([]model.ImageTag, 0, len(oldTags))

	oldTagMap := make(map[string]model.ImageTag, len(oldTags))

	for _, tag := range oldTags {
		oldTagMap[tag.Tag] = tag
	}

	for _, tag := range newTags {
		if _, exists := oldTagMap[tag.Tag]; exists {
			delete(oldTagMap, tag.Tag)
			continue
		}

		addedTags = append(addedTags, tag)
	}

	for _, tag := range oldTagMap {
		removedTags = append(removedTags, tag)
	}

	sort.Slice(removedTags, func(i, j int) bool {
		return removedTags[i].TagOrder < removedTags[j].TagOrder
	})

	return addedTags, removedTags
}

func tagNames(tags []model.ImageTag) []string {
	names := make([]string, 0, len(tags))

	for _, tag := range tags {
		names = append(names, tag.Tag)
	}

	return names
}
