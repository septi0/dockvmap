package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/store"
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
	ListImagesAfterID(ctx context.Context, afterID int64, limit int) ([]model.Image, error)
	CountImages(ctx context.Context, filters model.ImageListFilters) (int64, error)
	ImageStatusCounts(ctx context.Context) (model.ImageStatusCounts, error)
	UpdateImageCheck(ctx context.Context, tx store.DBTX, imageId int64, checkErr *string, checkedAt time.Time, tagSetHash string) (bool, error)
	TryStartImageRefresh(ctx context.Context, imageId int64) (bool, error)
	SetImageRefreshStatus(ctx context.Context, imageId int64, status string) error
	ResetRunningImageRefreshes(ctx context.Context) (int64, error)
	ListImageGroups(ctx context.Context, filters model.ImageGroupListFilters) ([]model.ImageGroup, error)
	CountImageGroups(ctx context.Context, filters model.ImageGroupListFilters) (int64, error)
	GetImageGroupByID(ctx context.Context, groupId int64) (*model.ImageGroup, error)
	ListImageGroupMembers(ctx context.Context, groupId int64) ([]model.Image, error)
	CreateImageGroup(ctx context.Context, tx store.DBTX, name string) (int64, error)
	UpdateImageGroupName(ctx context.Context, tx store.DBTX, groupId int64, name string) (bool, error)
	DeleteImageGroup(ctx context.Context, groupId int64) (bool, error)
	DeleteImageGroupIfEmpty(ctx context.Context, groupId int64) error
	UpdateImageGroupMembership(ctx context.Context, tx store.DBTX, imageId int64, groupId *int64) (bool, error)
	UpdateImageTag(ctx context.Context, tx store.DBTX, imageId int64, tag string) (bool, error)
	UpdateImageName(ctx context.Context, imageId int64, name string) (bool, error)
	UpdateImagePinned(ctx context.Context, tx store.DBTX, imageId int64, pinned bool) (bool, error)
	UpdateImageUpdateAvailable(ctx context.Context, tx store.DBTX, imageId int64, available bool, targetTag *string) (bool, error)
	GetImageTags(ctx context.Context, imageId int64) ([]model.ImageTag, error)
	GetImageTag(ctx context.Context, imageId int64, tag string) (*model.ImageTag, error)
	SetImageTags(ctx context.Context, tx store.DBTX, imageId int64, tags []model.ImageTag) error
	DeleteImageTagsNotSeen(ctx context.Context, tx store.DBTX, imageId int64, lastSeen time.Time) (int64, error)
	MarkImageTagsAsSeen(ctx context.Context, imageId int64) (int64, error)
	InsertImageTagHistory(ctx context.Context, tx store.DBTX, imageId int64, tag string, previousTag *string, source model.TagHistorySource) error
	GetImageTagHistory(ctx context.Context, imageId int64) ([]model.ImageTagHistory, error)
	UpdateImageDigest(ctx context.Context, imageId int64, tag string, digest *string, seenAt time.Time) error
}

type tagLister interface {
	ListTags(ctx context.Context, registry, repository string) ([]string, error)
}

type digestResolver interface {
	ManifestDigest(ctx context.Context, registry, repository, reference string) (string, error)
}

type failureRecorder interface {
	Record(ctx context.Context, source FailureSource, detail string, err error)
}

type tagFilterer interface {
	Apply(tags []string) []string
}

type Images struct {
	store         imageStore
	tagLister     tagLister
	digests       digestResolver
	events        eventHandler
	audit         auditRecorder
	failures      failureRecorder
	tagFilter     tagFilterer
	refreshLocker *imageRefreshLocker
	tagChanged    func(names ...string)
	bgCtx         context.Context
}

type imageChange struct {
	image              model.Image
	updateAvailable    bool
	updateAvailableTag string
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
	Group  string `json:"group,omitempty"`
}

type auditImageRenamedData struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

type auditImagePinChangedData struct {
	Name   string `json:"name"`
	Pinned bool   `json:"pinned"`
	Group  string `json:"group,omitempty"`
}

type ImagesDeps struct {
	Store     imageStore
	TagLister tagLister
	Digests   digestResolver
	Events    eventHandler
	Audit     auditRecorder
	Failures  failureRecorder
	TagFilter tagFilterer
	BgCtx     context.Context
}

func NewImages(deps ImagesDeps) *Images {
	return &Images{
		store:         deps.Store,
		tagLister:     deps.TagLister,
		digests:       deps.Digests,
		events:        deps.Events,
		audit:         deps.Audit,
		failures:      deps.Failures,
		tagFilter:     deps.TagFilter,
		refreshLocker: newImageRefreshLocker(),
		bgCtx:         deps.BgCtx,
	}
}

func (i *Images) OnTagChanged(fn func(names ...string)) {
	i.tagChanged = fn
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

	if !validRegistryAddress(img.Registry) {
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

func (i *Images) StatusCounts(ctx context.Context) (model.ImageStatusCounts, error) {
	return i.store.ImageStatusCounts(ctx)
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

	if image.GroupID != nil {
		if err := i.store.DeleteImageGroupIfEmpty(ctx, *image.GroupID); err != nil {
			slog.Error("removing empty image group failed", "group", *image.GroupID, "error", err)
		}
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

	tag = strings.TrimSpace(tag)

	if tag == "" {
		return fmt.Errorf("%w: tag is required", ErrInvalidImage)
	}

	if !tagRE.MatchString(tag) {
		return fmt.Errorf("%w: tag must be a valid tag", ErrInvalidImage)
	}

	images, unlock, err := i.lockImageScope(ctx, imageId)

	if err != nil {
		return err
	}

	defer unlock()

	return i.applyTag(ctx, images, tag, source)
}

func (i *Images) applyTag(ctx context.Context, images []model.Image, tag string, source model.TagHistorySource) error {
	changes := make([]imageChange, 0, len(images))
	missing := make([]string, 0)

	for _, image := range images {
		if image.Tag == tag {
			continue
		}

		allTags, err := i.store.GetImageTags(ctx, image.ID)

		if err != nil {
			return fmt.Errorf("checking available tags for %q: %w", image.Name, err)
		}

		if !hasImageTag(allTags, tag) {
			missing = append(missing, image.Name)

			continue
		}

		updateAvailable, updateAvailableTag := effectiveUpdateAvailable(image.Pinned, allTags, tag)

		changes = append(changes, imageChange{image, updateAvailable, updateAvailableTag})
	}

	if len(missing) > 0 {
		if images[0].GroupID == nil {
			return fmt.Errorf("%w: %q", ErrTagUnavailable, tag)
		}

		return &GroupTagUnavailableError{Tag: tag, Images: missing}
	}

	if len(changes) == 0 {
		return nil
	}

	tx, err := i.store.BeginTx(ctx)

	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, change := range changes {
		if err := i.writeTag(ctx, tx, change.image, tag, source); err != nil {
			return err
		}

		if err := i.writeUpdateAvailable(ctx, tx, &change.image, change.updateAvailable, change.updateAvailableTag); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	names := make([]string, 0, len(changes))

	for _, change := range changes {
		names = append(names, change.image.Name)

		i.auditTagChange(ctx, change.image, tag)
	}

	i.notifyTagChanged(names...)

	return nil
}

func (i *Images) writeTag(ctx context.Context, tx store.Transaction, image model.Image, tag string, source model.TagHistorySource) error {
	updated, err := i.store.UpdateImageTag(ctx, tx, image.ID, tag)

	if err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrImageNotFound, image.ID)
	}

	previousTag := image.Tag

	if err := i.store.InsertImageTagHistory(ctx, tx, image.ID, tag, &previousTag, source); err != nil {
		return fmt.Errorf("recording tag history for %q: %w", image.Name, err)
	}

	return nil
}

func (i *Images) auditTagChange(ctx context.Context, image model.Image, tag string) {
	recordAudit(ctx, i.audit, AuditTypeImageTagChanged, auditImageTagChangedData{
		Name:   image.Name,
		OldTag: image.Tag,
		NewTag: tag,
		Group:  groupName(image),
	})
}

func (i *Images) notifyTagChanged(names ...string) {
	if len(names) == 0 {
		return
	}

	if i.tagChanged != nil {
		i.tagChanged(names...)
	}

	i.refreshDigestsInBackground(names)
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

func (i *Images) SetPinned(ctx context.Context, imageId int64, pinned bool) error {
	if imageId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	images, unlock, err := i.lockImageScope(ctx, imageId)

	if err != nil {
		return err
	}

	defer unlock()

	changes := make([]imageChange, 0, len(images))

	for _, image := range images {
		if image.Pinned == pinned {
			continue
		}

		tags, err := i.store.GetImageTags(ctx, image.ID)

		if err != nil {
			return fmt.Errorf("getting tags for %q: %w", image.Name, err)
		}

		updateAvailable, updateAvailableTag := effectiveUpdateAvailable(pinned, tags, image.Tag)

		changes = append(changes, imageChange{image, updateAvailable, updateAvailableTag})
	}

	if len(changes) == 0 {
		return nil
	}

	tx, err := i.store.BeginTx(ctx)

	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, change := range changes {
		if _, err := i.store.UpdateImagePinned(ctx, tx, change.image.ID, pinned); err != nil {
			return fmt.Errorf("updating pinned state for %q: %w", change.image.Name, err)
		}

		if err := i.writeUpdateAvailable(ctx, tx, &change.image, change.updateAvailable, change.updateAvailableTag); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	for _, change := range changes {
		i.auditPinChange(ctx, change.image, pinned)
	}

	return nil
}

func (i *Images) auditPinChange(ctx context.Context, image model.Image, pinned bool) {
	recordAudit(ctx, i.audit, AuditTypeImagePinChanged, auditImagePinChangedData{
		Name:   image.Name,
		Pinned: pinned,
		Group:  groupName(image),
	})
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

	if image.GroupID != nil {
		return i.groupSelectableTags(ctx, *image.GroupID)
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

	if image.GroupID == nil {
		return i.store.MarkImageTagsAsSeen(ctx, imageId)
	}

	members, err := i.store.ListImageGroupMembers(ctx, *image.GroupID)

	if err != nil {
		return 0, err
	}

	var marked int64

	for _, member := range members {
		count, err := i.store.MarkImageTagsAsSeen(ctx, member.ID)

		if err != nil {
			return marked, err
		}

		marked += count
	}

	return marked, nil
}

func containsTag(tags []string, tag string) bool {
	return slices.Contains(tags, tag)
}

func hasImageTag(tags []model.ImageTag, tag string) bool {
	return slices.ContainsFunc(tags, func(t model.ImageTag) bool {
		return t.Tag == tag
	})
}
