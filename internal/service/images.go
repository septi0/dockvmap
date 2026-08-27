package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/taganalyzer"
)

var (
	ErrInvalidImage          = errors.New("invalid image")
	ErrImageAlreadyExists    = errors.New("image already exists")
	ErrImageNotFound         = errors.New("image not found")
	ErrTagUnavailable        = errors.New("image tag is not available")
	ErrTagRefreshFailed      = errors.New("tag refresh failed")
	ErrUpstreamNotFound      = errors.New("upstream repository not found")
	ErrUpstreamUnauthorized  = errors.New("upstream repository requires authentication")
	ErrUpstreamUnavailable   = errors.New("upstream registry unavailable")
	ErrFailedToRefreshTags   = errors.New("failed to refresh tags")
	ErrFailedToRegisterEvent = errors.New("failed to register event")
)

var (
	repositoryNameRE = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)
	tagRE            = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

type imageStore interface {
	BeginTx(ctx context.Context) (store.Transaction, error)
	CreateImage(ctx context.Context, img *model.Image) error
	DeleteImage(ctx context.Context, imageId int64) (bool, error)
	GetImageByID(ctx context.Context, imageId int64) (*model.Image, error)
	GetImage(ctx context.Context, name string) (*model.Image, error)
	GetRegistryInfoByID(ctx context.Context, registryID int64) (*model.RegistryInfo, error)
	ListImages(ctx context.Context, filters model.ImageListFilters) ([]model.Image, error)
	CountImages(ctx context.Context, filters model.ImageListFilters) (int64, error)
	UpdateImageCheck(ctx context.Context, tx store.DBTX, imageId int64, checkErr *string, checkedAt time.Time) (bool, error)
	UpdateImageTag(ctx context.Context, tx store.DBTX, imageId int64, tag string) (bool, error)
	UpdateImageName(ctx context.Context, imageId int64, name string) (bool, error)
	UpdateImageUpdateAvailable(ctx context.Context, tx store.DBTX, imageId int64, available bool, targetTag *string) (bool, error)
	GetImageTags(ctx context.Context, imageId int64) ([]model.ImageTag, error)
	GetImageTag(ctx context.Context, imageId int64, tag string) (*model.ImageTag, error)
	SetImageTags(ctx context.Context, tx store.DBTX, imageId int64, tags []model.ImageTag) error
	DeleteImageTagsNotSeen(ctx context.Context, tx store.DBTX, imageId int64, lastSeen time.Time) (int64, error)
	MarkImageTagsAsSeen(ctx context.Context, imageId int64) (int64, error)
	InsertImageTagHistory(ctx context.Context, tx store.DBTX, imageId int64, tag string, previousTag *string, source model.TagHistorySource) error
	GetImageTagHistory(ctx context.Context, imageId int64) ([]model.ImageTagHistory, error)
}

type tagLister interface {
	ListTags(ctx context.Context, registry, repository string) ([]string, error)
}

type failureRecorder interface {
	Record(source FailureSource, detail string, err error)
}

type tagFilterer interface {
	Apply(tags []string) []string
}

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

func (l *imageRefreshLocker) lock(imageID int64) func() {
	l.mu.Lock()

	lock, ok := l.locks[imageID]

	if !ok {
		lock = &imageRefreshLock{}
		l.locks[imageID] = lock
	}

	lock.refs++

	l.mu.Unlock()

	lock.mu.Lock()

	return func() {
		lock.mu.Unlock()

		l.mu.Lock()

		lock.refs--

		if lock.refs == 0 {
			delete(l.locks, imageID)
		}

		l.mu.Unlock()
	}
}

type Images struct {
	store         imageStore
	tagLister     tagLister
	events        eventHandler
	audit         auditRecorder
	failures      failureRecorder
	tagFilter     tagFilterer
	refreshLocker *imageRefreshLocker
}

type RefreshTagsOpts struct {
	FlagAsNew     bool
	RegisterEvent EventMode
	Tags          []string // already fetched and filtered; if nil, tags are fetched and filtered internally
}

type auditImageData struct {
	Name       string `json:"name"`
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

type auditImageTagChangedData struct {
	Name   string `json:"name"`
	OldTag string `json:"oldTag"`
	NewTag string `json:"newTag"`
}

type auditImageRenamedData struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

func NewImages(store imageStore, tagLister tagLister, events eventHandler, audit auditRecorder, failures failureRecorder, tagFilter tagFilterer) *Images {
	return &Images{
		store:         store,
		tagLister:     tagLister,
		events:        events,
		audit:         audit,
		failures:      failures,
		tagFilter:     tagFilter,
		refreshLocker: newImageRefreshLocker(),
	}
}

func (i *Images) Create(ctx context.Context, img model.Image, availableTags []string) error {
	img.Name = strings.TrimSpace(img.Name)
	img.Repository = strings.TrimSpace(img.Repository)
	img.Tag = strings.TrimSpace(img.Tag)

	if img.Tag == "" {
		img.Tag = "latest"
	}

	if img.Name == "" || img.Repository == "" {
		return fmt.Errorf("%w: name and repository are required", ErrInvalidImage)
	}

	if img.RegistryID <= 0 {
		return fmt.Errorf("%w: registry id is required", ErrInvalidImage)
	}

	if !repositoryNameRE.MatchString(img.Name) || !repositoryNameRE.MatchString(img.Repository) {
		return fmt.Errorf("%w: name and repository must be valid lowercase repository paths", ErrInvalidImage)
	}

	registryInfo, err := i.store.GetRegistryInfoByID(ctx, img.RegistryID)

	if err != nil {
		return err
	}

	if registryInfo == nil {
		return fmt.Errorf("%w: registry does not exist", ErrInvalidImage)
	}

	img.Registry = registryInfo.Registry

	if !validRegistry(img.Registry) {
		return fmt.Errorf("%w: registry must be a valid host", ErrInvalidImage)
	}

	if !tagRE.MatchString(img.Tag) {
		return fmt.Errorf("%w: tag must be a valid tag", ErrInvalidImage)
	}

	if availableTags == nil {
		tags, err := i.tagLister.ListTags(ctx, img.Registry, img.Repository)

		if err != nil {
			var registryErr *oci.Error

			if errors.As(err, &registryErr) {
				switch registryErr.StatusCode {
				case http.StatusNotFound:
					return fmt.Errorf("%w: %s/%s", ErrUpstreamNotFound, img.Registry, img.Repository)

				case http.StatusUnauthorized:
					return fmt.Errorf("%w: %s/%s", ErrUpstreamUnauthorized, img.Registry, img.Repository)
				}
			}

			return fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
		}

		availableTags = i.tagFilter.Apply(tags)
	}

	if !containsTag(availableTags, img.Tag) {
		return fmt.Errorf("%w: %q", ErrTagUnavailable, img.Tag)
	}

	if err := i.store.CreateImage(ctx, &img); err != nil {
		if errors.Is(err, store.ErrImageNameConflict) {
			return ErrImageAlreadyExists
		}

		return err
	}

	recordAudit(ctx, i.audit, AuditTypeImageCreated, auditImageData{
		Name:       img.Name,
		Registry:   img.Registry,
		Repository: img.Repository,
		Tag:        img.Tag,
	})

	if err := i.store.InsertImageTagHistory(ctx, nil, img.ID, img.Tag, nil, model.TagHistorySourceCreated); err != nil {
		slog.Error("recording initial tag history failed", "image", img.Name, "error", err)
	}

	opts := RefreshTagsOpts{
		FlagAsNew:     false,
		RegisterEvent: EventNone,
		Tags:          availableTags,
	}

	if err := i.RefreshAvailableTags(ctx, img.ID, opts); err != nil {
		return ErrFailedToRefreshTags
	}

	return nil
}

func (i *Images) List(ctx context.Context, filters model.ImageListFilters) ([]model.Image, error) {
	return i.store.ListImages(ctx, filters)
}

func (i *Images) Count(ctx context.Context, filters model.ImageListFilters) (int64, error) {
	return i.store.CountImages(ctx, filters)
}

func (i *Images) GetByID(ctx context.Context, imageId int64) (*model.Image, error) {
	if imageId < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return nil, err
	}

	if image == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	return image, nil
}

func (i *Images) Delete(ctx context.Context, imageId int64) (bool, error) {
	if imageId < 1 {
		return false, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return false, err
	}

	if image == nil {
		return false, nil
	}

	deleted, err := i.store.DeleteImage(ctx, imageId)

	if err != nil || !deleted {
		return deleted, err
	}

	recordAudit(ctx, i.audit, AuditTypeImageDeleted, auditImageData{
		Name:       image.Name,
		Registry:   image.Registry,
		Repository: image.Repository,
		Tag:        image.Tag,
	})

	return true, nil
}

func (i *Images) UpdateTag(ctx context.Context, imageId int64, tag string, source model.TagHistorySource) error {
	if imageId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	unlock := i.refreshLocker.lock(imageId)
	defer unlock()

	tag = strings.TrimSpace(tag)

	if tag == "" {
		return fmt.Errorf("%w: tag is required", ErrInvalidImage)
	}

	if !tagRE.MatchString(tag) {
		return fmt.Errorf("%w: tag must be a valid tag", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return err
	}

	if image == nil {
		return fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	if tag == image.Tag {
		return nil
	}

	if image_tag, err := i.store.GetImageTag(ctx, imageId, tag); err != nil {
		return fmt.Errorf("checking available tags for %q: %w", image.Name, err)
	} else if image_tag == nil {
		return fmt.Errorf("%w: %q", ErrTagUnavailable, tag)
	}

	allTags, err := i.store.GetImageTags(ctx, imageId)

	if err != nil {
		return fmt.Errorf("checking available tags for %q: %w", image.Name, err)
	}

	updateAvailable, updateAvailableTag := updateAvailableFor(allTags, tag)

	tx, err := i.store.BeginTx(ctx)

	if err != nil {
		return err
	}

	defer tx.Rollback()

	updated, err := i.store.UpdateImageTag(ctx, tx, imageId, tag)

	if err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	previousTag := image.Tag

	if err := i.store.InsertImageTagHistory(ctx, tx, imageId, tag, &previousTag, source); err != nil {
		return fmt.Errorf("recording tag history for %q: %w", image.Name, err)
	}

	if err := i.commitUpdateAvailable(ctx, tx, image, updateAvailable, updateAvailableTag); err != nil {
		return err
	}

	recordAudit(ctx, i.audit, AuditTypeImageTagChanged, auditImageTagChangedData{
		Name:   image.Name,
		OldTag: image.Tag,
		NewTag: tag,
	})

	return nil
}

func (i *Images) GetTagHistory(ctx context.Context, imageId int64) ([]model.ImageTagHistory, error) {
	if imageId < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return nil, err
	}

	if image == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	return i.store.GetImageTagHistory(ctx, imageId)
}

func (i *Images) Rename(ctx context.Context, imageId int64, name string) error {
	if imageId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	unlock := i.refreshLocker.lock(imageId)
	defer unlock()

	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidImage)
	}

	if !repositoryNameRE.MatchString(name) {
		return fmt.Errorf("%w: name must be a valid lowercase repository path", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return err
	}

	if image == nil {
		return fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	if name == image.Name {
		return nil
	}

	updated, err := i.store.UpdateImageName(ctx, imageId, name)

	if err != nil {
		if errors.Is(err, store.ErrImageNameConflict) {
			return ErrImageAlreadyExists
		}

		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	recordAudit(ctx, i.audit, AuditTypeImageRenamed, auditImageRenamedData{
		OldName: image.Name,
		NewName: name,
	})

	return nil
}

func (i *Images) commitUpdateAvailable(ctx context.Context, tx store.Transaction, image *model.Image, updateAvailable bool, updateAvailableTag string) error {
	var targetTag *string
	if updateAvailableTag != "" {
		targetTag = &updateAvailableTag
	}

	if _, err := i.store.UpdateImageUpdateAvailable(ctx, tx, image.ID, updateAvailable, targetTag); err != nil {
		return fmt.Errorf("updating update-available flag for %q: %w", image.Name, err)
	}

	return tx.Commit()
}

func updateAvailableFor(tags []model.ImageTag, currentTag string) (bool, string) {
	var current *model.ImageTag

	for i := range tags {
		if tags[i].Tag == currentTag {
			current = &tags[i]

			break
		}
	}

	if current == nil || !current.FamilyHasOrder {
		return false, ""
	}

	var target *model.ImageTag

	for i := range tags {
		t := &tags[i]

		if t.FamilyID != current.FamilyID || t.TagOrder >= current.TagOrder {
			continue
		}

		if !current.Prerelease && t.Prerelease {
			continue
		}

		if target == nil || t.TagOrder < target.TagOrder {
			target = t
		}
	}

	if target == nil {
		return false, ""
	}

	return true, target.Tag
}

func (i *Images) RefreshAvailableTags(ctx context.Context, imageId int64, options RefreshTagsOpts) error {
	if imageId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	unlock := i.refreshLocker.lock(imageId)
	defer unlock()

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

			i.failures.Record(FailureSourceRefresh, image.Name, err)

			if _, updateErr := i.store.UpdateImageCheck(ctx, nil, image.ID, &message, checkedAt); updateErr != nil {
				return fmt.Errorf("checking tags: %v; recording check failure: %w", err, updateErr)
			}

			return fmt.Errorf("%w for %q: %w", ErrTagRefreshFailed, image.Name, err)
		}

		sourceTags = i.tagFilter.Apply(sourceTags)
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

	if _, err := i.store.UpdateImageCheck(ctx, tx, image.ID, nil, checkedAt); err != nil {
		return fmt.Errorf("recording tag check for %q: %w", image.Name, err)
	}

	updateAvailable, updateAvailableTag := updateAvailableFor(tags, image.Tag)

	if err := i.commitUpdateAvailable(ctx, tx, image, updateAvailable, updateAvailableTag); err != nil {
		return err
	}

	if err := i.events.HandleEvent(ctx, options.RegisterEvent, image, oldTags, tags); err != nil {
		return fmt.Errorf("%w: registering image event for %q: %w", ErrFailedToRegisterEvent, image.Name, err)
	}

	return nil
}

func (i *Images) RefreshAll(ctx context.Context) (int, error) {
	batchSize := 100
	offset := 0
	refreshed := 0

	var refreshErrors []error

	for {
		images, err := i.List(ctx, model.ImageListFilters{Pagination: model.Pagination{Offset: offset, Limit: batchSize}})

		if err != nil {
			return refreshed, fmt.Errorf("listing virtual images: %w", err)
		}

		if len(images) == 0 {
			break
		}

		offset += len(images)

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
				i.failures.Record(FailureSourceEventRegistration, image.Name, err)
				refreshed++

			default:
				refreshErrors = append(refreshErrors, fmt.Errorf("refreshing image %d: %w", image.ID, err))
			}
		}
	}

	return refreshed, errors.Join(refreshErrors...)
}

func (i *Images) Resolve(ctx context.Context, name string) (*model.Image, error) {
	return i.store.GetImage(ctx, name)
}

func (i *Images) GetTags(ctx context.Context, imageId int64) ([]model.ImageTag, error) {
	if imageId < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return nil, err
	}

	if image == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	return i.store.GetImageTags(ctx, imageId)
}

func (i *Images) MarkTagsAsSeen(ctx context.Context, imageId int64) (int64, error) {
	if imageId < 1 {
		return 0, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return 0, err
	}

	if image == nil {
		return 0, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	return i.store.MarkImageTagsAsSeen(ctx, imageId)
}

func containsTag(tags []string, tag string) bool {
	return slices.Contains(tags, tag)
}

func validRegistry(value string) bool {
	parsed, err := url.Parse("https://" + value)

	if err != nil || parsed.Host != value || parsed.User != nil || parsed.Path != "" {
		return false
	}

	host, _, err := net.SplitHostPort(value)

	if err == nil {
		return host != ""
	}

	return !strings.Contains(value, ":")
}

func imageTagsFromAnalysis(analysis taganalyzer.Analysis, imageID int64, seenAt time.Time, flagAsNew bool) []model.ImageTag {
	var tags []model.ImageTag
	order := 0

	prerelease := make(map[string]bool, len(analysis.Tags))
	for _, tag := range analysis.Tags {
		prerelease[tag.Tag] = taganalyzer.IsPrerelease(tag)
	}

	for _, family := range analysis.Ordered {
		for _, tag := range family.OrderedTags {
			tags = append(tags, model.ImageTag{
				ImageID:        imageID,
				FamilyID:       family.ID,
				FamilyType:     string(family.Kind),
				FamilyHasOrder: family.HasOrder,
				Tag:            tag,
				TagOrder:       order,
				Prerelease:     prerelease[tag],
				FirstSeen:      seenAt,
				LastSeen:       seenAt,
				New:            flagAsNew,
			})

			order++
		}
	}

	return tags
}
