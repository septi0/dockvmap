package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/taganalyzer"
)

type imageRefreshLock struct {
	mu   sync.Mutex
	refs int
}

type imageRefreshLocker struct {
	mu    sync.Mutex
	locks map[int64]*imageRefreshLock
}

func newImageRefreshLocker() *imageRefreshLocker {
	return &imageRefreshLocker{
		locks: make(map[int64]*imageRefreshLock),
	}
}

func (l *imageRefreshLocker) acquire(imageID int64) *imageRefreshLock {
	l.mu.Lock()
	defer l.mu.Unlock()

	lock, ok := l.locks[imageID]

	if !ok {
		lock = &imageRefreshLock{}
		l.locks[imageID] = lock
	}

	lock.refs++

	return lock
}

func (l *imageRefreshLocker) release(imageID int64, lock *imageRefreshLock) {
	l.mu.Lock()
	defer l.mu.Unlock()

	lock.refs--

	if lock.refs == 0 {
		delete(l.locks, imageID)
	}
}

func (l *imageRefreshLocker) lock(imageID int64) func() {
	lock := l.acquire(imageID)

	lock.mu.Lock()

	return func() {
		lock.mu.Unlock()

		l.release(imageID, lock)
	}
}

func (l *imageRefreshLocker) tryLockAll(imageIDs []int64) (func(), bool) {
	unlocks := make([]func(), 0, len(imageIDs))

	unlockAll := func() {
		for _, unlock := range unlocks {
			unlock()
		}
	}

	for _, imageID := range imageIDs {
		lock := l.acquire(imageID)

		if !lock.mu.TryLock() {
			l.release(imageID, lock)
			unlockAll()

			return nil, false
		}

		unlocks = append(unlocks, func() {
			lock.mu.Unlock()

			l.release(imageID, lock)
		})
	}

	return unlockAll, true
}

func (l *imageRefreshLocker) lockAll(ctx context.Context, imageIDs []int64) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, imageGroupLockWait)
	defer cancel()

	ticker := time.NewTicker(imageGroupLockRetryInterval)
	defer ticker.Stop()

	for {
		if unlock, ok := l.tryLockAll(imageIDs); ok {
			return unlock, nil
		}

		select {
		case <-ctx.Done():
			return nil, ErrImageGroupBusy

		case <-ticker.C:
		}
	}
}

type RefreshTagsOpts struct {
	FlagAsNew     bool
	RegisterEvent EventMode
	Tags          []string // already fetched and filtered; if nil, tags are fetched and filtered internally
}

func (i *Images) commitUpdateAvailable(ctx context.Context, tx store.Transaction, image *model.Image, updateAvailable bool, updateAvailableTag string) error {
	if err := i.writeUpdateAvailable(ctx, tx, image, updateAvailable, updateAvailableTag); err != nil {
		return err
	}

	return tx.Commit()
}

func (i *Images) writeUpdateAvailable(ctx context.Context, tx store.DBTX, image *model.Image, updateAvailable bool, updateAvailableTag string) error {
	var targetTag *string
	if updateAvailableTag != "" {
		targetTag = &updateAvailableTag
	}

	if _, err := i.store.UpdateImageUpdateAvailable(ctx, tx, image.ID, updateAvailable, targetTag); err != nil {
		return fmt.Errorf("updating update-available flag for %q: %w", image.Name, err)
	}

	return nil
}

func effectiveUpdateAvailable(pinned bool, tags []model.ImageTag, currentTag string) (bool, string) {
	if pinned {
		return false, ""
	}

	return updateAvailableFor(tags, currentTag)
}

func updateAvailableFor(tags []model.ImageTag, currentTag string) (bool, string) {
	candidates := upgradeCandidates(tags, currentTag)

	if len(candidates) == 0 {
		return false, ""
	}

	return true, candidates[0].Tag
}

func upgradeCandidates(tags []model.ImageTag, currentTag string) []model.ImageTag {
	var current *model.ImageTag

	for i := range tags {
		if tags[i].Tag == currentTag {
			current = &tags[i]

			break
		}
	}

	if current == nil || !current.FamilyHasOrder {
		return nil
	}

	var candidates []model.ImageTag

	for _, t := range tags {
		if t.FamilyID != current.FamilyID || t.TagOrder >= current.TagOrder {
			continue
		}

		if !current.Prerelease && t.Prerelease {
			continue
		}

		if versionContains(current.VersionSegments, t.VersionSegments) {
			continue
		}

		candidates = append(candidates, t)
	}

	slices.SortFunc(candidates, func(a, b model.ImageTag) int {
		return a.TagOrder - b.TagOrder
	})

	return candidates
}

func CurrentAndNewerFamilyTags(tags []model.ImageTag, currentTag string) []model.ImageTag {
	family := make([]model.ImageTag, 0)

	var current *model.ImageTag

	for i := range tags {
		if tags[i].Tag == currentTag {
			current = &tags[i]

			break
		}
	}

	if current == nil {
		return family
	}

	for _, tag := range tags {
		if tag.FamilyID == current.FamilyID && tag.TagOrder <= current.TagOrder {
			family = append(family, tag)
		}
	}

	return family
}

func versionContains(outer, inner [][]int64) bool {
	if len(outer) == 0 || len(outer) > len(inner) {
		return false
	}

	moreSpecific := len(inner) > len(outer)

	for i := range outer {
		if !numberPrefix(outer[i], inner[i]) {
			return false
		}

		if len(outer[i]) < len(inner[i]) {
			moreSpecific = true
		}
	}

	return moreSpecific
}

func numberPrefix(prefix, full []int64) bool {
	if len(prefix) > len(full) {
		return false
	}

	for i := range prefix {
		if prefix[i] != full[i] {
			return false
		}
	}

	return true
}

func (i *Images) RefreshAvailableTags(ctx context.Context, imageId int64, options RefreshTagsOpts) error {
	if imageId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	unlock := i.refreshLocker.lock(imageId)
	defer unlock()

	ctx, cancel := context.WithTimeout(ctx, backgroundScanTimeout)
	defer cancel()

	if err := i.store.SetImageRefreshStatus(ctx, imageId, model.RefreshStatusRunning); err != nil {
		slog.Error("marking image refresh in progress failed", "imageId", imageId, "error", err)
	}

	defer func() {
		resetCtx, resetCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer resetCancel()

		if err := i.store.SetImageRefreshStatus(resetCtx, imageId, model.RefreshStatusIdle); err != nil {
			slog.Error("clearing image refresh status failed", "imageId", imageId, "error", err)
		}
	}()

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return err
	}

	if image == nil {
		return fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	checkedAt := time.Now().UTC()

	sourceTags := options.Tags

	if options.Tags == nil {
		sourceTags, err = i.tagLister.ListTags(ctx, image.Registry, image.Repository)

		if err != nil {
			message := err.Error()

			i.failures.Record(ctx, FailureSourceRefresh, image.Name, err)

			if _, updateErr := i.store.UpdateImageCheck(ctx, nil, image.ID, &message, checkedAt, image.TagSetHash); updateErr != nil {
				return fmt.Errorf("checking tags: %v; recording check failure: %w", err, updateErr)
			}

			return fmt.Errorf("%w for %q: %w", ErrTagRefreshFailed, image.Name, err)
		}

		sourceTags = i.tagFilter.Apply(sourceTags)
	}

	tagsHash := tagSetHash(sourceTags)

	if tagsHash == image.TagSetHash {
		if _, err := i.store.UpdateImageCheck(ctx, nil, image.ID, nil, checkedAt, tagsHash); err != nil {
			return fmt.Errorf("recording tag check for %q: %w", image.Name, err)
		}

		return nil
	}

	analyzedTags := taganalyzer.Analyze(sourceTags)

	tags := imageTagsFromAnalysis(analyzedTags, image.ID, checkedAt, options.FlagAsNew)

	oldTags, err := i.store.GetImageTags(ctx, image.ID)

	if err != nil {
		return fmt.Errorf("getting old tags for %q: %w", image.Name, err)
	}

	tx, err := i.store.BeginTx(ctx)

	if err != nil {
		return err
	}

	defer tx.Rollback()

	if err := i.store.SetImageTags(ctx, tx, image.ID, tags); err != nil {
		return fmt.Errorf("updating available tags for %q: %w", image.Name, err)
	}

	if _, err := i.store.DeleteImageTagsNotSeen(ctx, tx, image.ID, checkedAt); err != nil {
		return fmt.Errorf("deleting not seen tags for %q: %w", image.Name, err)
	}

	if _, err := i.store.UpdateImageCheck(ctx, tx, image.ID, nil, checkedAt, tagsHash); err != nil {
		return fmt.Errorf("recording tag check for %q: %w", image.Name, err)
	}

	updateAvailable, updateAvailableTag := effectiveUpdateAvailable(image.Pinned, tags, image.Tag)

	if err := i.commitUpdateAvailable(ctx, tx, image, updateAvailable, updateAvailableTag); err != nil {
		return err
	}

	if err := i.events.HandleEvent(ctx, options.RegisterEvent, image, oldTags, tags, updateAvailable, updateAvailableTag); err != nil {
		return fmt.Errorf("%w: registering image event for %q: %w", ErrFailedToRegisterEvent, image.Name, err)
	}

	return nil
}

func (i *Images) StartBackgroundRefresh(imageId int64) (bool, error) {
	if imageId < 1 {
		return false, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(i.bgCtx, imageId)

	if err != nil {
		return false, err
	}

	if image == nil {
		return false, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	started, err := i.store.TryStartImageRefresh(i.bgCtx, imageId)

	if err != nil {
		return false, err
	}

	if !started {
		return false, nil
	}

	imageIDs := []int64{imageId}

	if image.GroupID != nil {
		members, err := i.store.ListImageGroupMembers(i.bgCtx, *image.GroupID)

		if err != nil {
			slog.Error("listing group members for refresh failed", "group", *image.GroupID, "error", err)
		}

		imageIDs = groupRefreshOrder(members, imageId)
	}

	go func() {
		opts := RefreshTagsOpts{
			FlagAsNew:     true,
			RegisterEvent: EventSilent,
		}

		for _, id := range imageIDs {
			if err := i.RefreshAvailableTags(i.bgCtx, id, opts); err != nil {
				slog.Error("background tag refresh failed", "imageId", id, "error", err)
			}
		}
	}()

	return true, nil
}

func (i *Images) RecoverFromRestart(ctx context.Context) error {
	count, err := i.store.ResetRunningImageRefreshes(ctx)

	if err != nil {
		return fmt.Errorf("recovering image refresh state: %w", err)
	}

	if count > 0 {
		slog.Warn("reset interrupted image refreshes", "count", count)
	}

	return nil
}

func (i *Images) RefreshAll(ctx context.Context) (int, error) {
	const batchSize = 100

	var afterID int64
	refreshed := 0

	var refreshErrors []error

	for {
		images, err := i.store.ListImagesAfterID(ctx, afterID, batchSize)

		if err != nil {
			return refreshed, fmt.Errorf("listing virtual images: %w", err)
		}

		if len(images) == 0 {
			break
		}

		afterID = images[len(images)-1].ID

		for _, image := range images {
			if err := ctx.Err(); err != nil {
				return refreshed, err
			}

			opts := RefreshTagsOpts{
				FlagAsNew:     true,
				RegisterEvent: EventNormal,
			}

			err := i.RefreshAvailableTags(ctx, image.ID, opts)

			switch {
			case err == nil:
				refreshed++

			case errors.Is(err, ErrFailedToRegisterEvent):
				i.failures.Record(ctx, FailureSourceEventRegistration, image.Name, err)
				refreshed++

			default:
				refreshErrors = append(refreshErrors, fmt.Errorf("refreshing image %d: %w", image.ID, err))
			}
		}
	}

	return refreshed, errors.Join(refreshErrors...)
}

func tagSetHash(tags []string) string {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)

	h := sha256.New()

	fmt.Fprintf(h, "e%d\x00", taganalyzer.AnalysisEpoch)

	for _, tag := range sorted {
		h.Write([]byte(tag))
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

func versionSegments(tag taganalyzer.TagAnalysis) [][]int64 {
	var segments [][]int64

	for _, segment := range tag.Segments {
		if segment.OrderType == taganalyzer.OrderVersion && len(segment.Numbers) > 0 {
			segments = append(segments, segment.Numbers)
		}
	}

	return segments
}

func imageTagsFromAnalysis(analysis taganalyzer.Analysis, imageID int64, seenAt time.Time, flagAsNew bool) []model.ImageTag {
	var tags []model.ImageTag
	order := 0

	prerelease := make(map[string]bool, len(analysis.Tags))
	versions := make(map[string][][]int64, len(analysis.Tags))
	for _, tag := range analysis.Tags {
		prerelease[tag.Tag] = taganalyzer.IsPrerelease(tag)
		versions[tag.Tag] = versionSegments(tag)
	}

	for _, family := range analysis.Ordered {
		for _, tag := range family.OrderedTags {
			tags = append(tags, model.ImageTag{
				ImageID:         imageID,
				FamilyID:        family.ID,
				FamilyType:      string(family.Kind),
				FamilyHasOrder:  family.HasOrder,
				Tag:             tag,
				TagOrder:        order,
				Prerelease:      prerelease[tag],
				VersionSegments: versions[tag],
				FirstSeen:       seenAt,
				LastSeen:        seenAt,
				New:             flagAsNew,
			})

			order++
		}
	}

	return tags
}
