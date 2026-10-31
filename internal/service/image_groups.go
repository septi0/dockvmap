package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

const (
	imageGroupMinImages         = 2
	imageGroupNameMaxLength     = 100
	imageGroupLockWait          = 5 * time.Second
	imageGroupLockRetryInterval = 100 * time.Millisecond
)

var (
	ErrInvalidImageGroup       = errors.New("invalid image group")
	ErrImageGroupNotFound      = errors.New("image group not found")
	ErrImageGroupAlreadyExists = errors.New("image group already exists")
	ErrImageGroupBusy          = errors.New("a tag refresh is running for this group, try again shortly")
	ErrImageInAnotherGroup     = errors.New("image already belongs to another group")
)

type GroupTagUnavailableError struct {
	Tag    string
	Images []string
}

func (e *GroupTagUnavailableError) Error() string {
	return fmt.Sprintf("tag %q is not available on: %s", e.Tag, strings.Join(e.Images, ", "))
}

func (e *GroupTagUnavailableError) Unwrap() error {
	return ErrTagUnavailable
}

type ImageGroupInput struct {
	Name     string
	ImageIDs []int64
	Tag      string
}

type auditImageGroupData struct {
	Name    string   `json:"name"`
	OldName string   `json:"oldName,omitempty"`
	Images  []string `json:"images"`
	Tag     string   `json:"tag,omitempty"`
}

type imageGroupMember struct {
	image model.Image
	tags  []model.ImageTag
}

func groupName(image model.Image) string {
	if image.GroupName == nil {
		return ""
	}

	return *image.GroupName
}

func imageIDs(images []model.Image) []int64 {
	ids := make([]int64, 0, len(images))

	for _, image := range images {
		ids = append(ids, image.ID)
	}

	return ids
}

func imageNames(images []model.Image) []string {
	names := make([]string, 0, len(images))

	for _, image := range images {
		names = append(names, image.Name)
	}

	return names
}

func groupRefreshOrder(members []model.Image, imageId int64) []int64 {
	ids := make([]int64, 0, len(members)+1)

	for _, member := range members {
		if member.ID != imageId {
			ids = append(ids, member.ID)
		}
	}

	return append(ids, imageId)
}

func (i *Images) imageScope(ctx context.Context, imageId int64) ([]model.Image, error) {
	image, err := i.store.GetImageByID(ctx, imageId)

	if err != nil {
		return nil, err
	}

	if image == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageNotFound, imageId)
	}

	if image.GroupID == nil {
		return []model.Image{*image}, nil
	}

	return i.store.ListImageGroupMembers(ctx, *image.GroupID)
}

func (i *Images) lockImageScope(ctx context.Context, imageId int64) ([]model.Image, func(), error) {
	for {
		scope, err := i.imageScope(ctx, imageId)

		if err != nil {
			return nil, nil, err
		}

		ids := imageIDs(scope)

		var unlock func()

		if len(ids) == 1 {
			unlock = i.refreshLocker.lock(ids[0])
		} else if unlock, err = i.refreshLocker.lockAll(ctx, ids); err != nil {
			return nil, nil, err
		}

		locked, err := i.imageScope(ctx, imageId)

		if err != nil {
			unlock()

			return nil, nil, err
		}

		if slices.Equal(ids, imageIDs(locked)) {
			return locked, unlock, nil
		}

		unlock()
	}
}

func (i *Images) loadGroupMembers(ctx context.Context, images []model.Image) ([]imageGroupMember, error) {
	members := make([]imageGroupMember, 0, len(images))

	for _, image := range images {
		tags, err := i.store.GetImageTags(ctx, image.ID)

		if err != nil {
			return nil, fmt.Errorf("getting tags for %q: %w", image.Name, err)
		}

		members = append(members, imageGroupMember{image: image, tags: tags})
	}

	return members, nil
}

func (i *Images) populateGroup(ctx context.Context, group *model.ImageGroup) error {
	images, err := i.store.ListImageGroupMembers(ctx, group.ID)

	if err != nil {
		return err
	}

	members, err := i.loadGroupMembers(ctx, images)

	if err != nil {
		return err
	}

	group.Images = images

	applyGroupState(group, members)

	return nil
}

func (i *Images) ListGroups(ctx context.Context, filters model.ImageGroupListFilters) ([]model.ImageGroup, error) {
	groups, err := i.store.ListImageGroups(ctx, filters)

	if err != nil {
		return nil, err
	}

	for index := range groups {
		if err := i.populateGroup(ctx, &groups[index]); err != nil {
			return nil, err
		}
	}

	return groups, nil
}

func (i *Images) CountGroups(ctx context.Context, filters model.ImageGroupListFilters) (int64, error) {
	return i.store.CountImageGroups(ctx, filters)
}

func (i *Images) GetGroup(ctx context.Context, groupId int64) (*model.ImageGroup, error) {
	if groupId < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidImageGroup)
	}

	group, err := i.store.GetImageGroupByID(ctx, groupId)

	if err != nil {
		return nil, err
	}

	if group == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageGroupNotFound, groupId)
	}

	if err := i.populateGroup(ctx, group); err != nil {
		return nil, err
	}

	return group, nil
}

func (i *Images) CommonTags(ctx context.Context, ids []int64) ([]model.ImageTag, error) {
	images, err := i.getImages(ctx, uniqueIDs(ids))

	if err != nil {
		return nil, err
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("%w: at least one image is required", ErrInvalidImageGroup)
	}

	members, err := i.loadGroupMembers(ctx, images)

	if err != nil {
		return nil, err
	}

	return commonTags(members, members[0], ""), nil
}

func (i *Images) groupSelectableTags(ctx context.Context, groupId int64) ([]model.ImageTag, error) {
	images, err := i.store.ListImageGroupMembers(ctx, groupId)

	if err != nil {
		return nil, err
	}

	members, err := i.loadGroupMembers(ctx, images)

	if err != nil {
		return nil, err
	}

	if len(members) == 0 {
		return []model.ImageTag{}, nil
	}

	lead, _ := groupLead(members, tagPresence(members))

	return commonTags(members, lead, members[0].image.Tag), nil
}

func (i *Images) getImages(ctx context.Context, ids []int64) ([]model.Image, error) {
	images := make([]model.Image, 0, len(ids))

	for _, id := range ids {
		image, err := i.store.GetImageByID(ctx, id)

		if err != nil {
			return nil, err
		}

		if image == nil {
			return nil, fmt.Errorf("%w: %d", ErrImageNotFound, id)
		}

		images = append(images, *image)
	}

	return images, nil
}

func (i *Images) CreateGroup(ctx context.Context, input ImageGroupInput) (int64, error) {
	return i.saveGroup(ctx, 0, input)
}

func (i *Images) UpdateGroup(ctx context.Context, groupId int64, input ImageGroupInput) error {
	if groupId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImageGroup)
	}

	_, err := i.saveGroup(ctx, groupId, input)

	return err
}

func (i *Images) saveGroup(ctx context.Context, groupId int64, input ImageGroupInput) (int64, error) {
	input, err := normalizeGroupInput(input)

	if err != nil {
		return 0, err
	}

	var group *model.ImageGroup
	var previous []model.Image

	if groupId > 0 {
		if group, err = i.store.GetImageGroupByID(ctx, groupId); err != nil {
			return 0, err
		}

		if group == nil {
			return 0, fmt.Errorf("%w: %d", ErrImageGroupNotFound, groupId)
		}

		if previous, err = i.store.ListImageGroupMembers(ctx, groupId); err != nil {
			return 0, err
		}
	}

	lockIDs := uniqueIDs(append(imageIDs(previous), input.ImageIDs...))

	unlock, err := i.refreshLocker.lockAll(ctx, lockIDs)

	if err != nil {
		return 0, err
	}

	defer unlock()

	if groupId > 0 {
		current, err := i.store.ListImageGroupMembers(ctx, groupId)

		if err != nil {
			return 0, err
		}

		if !slices.Equal(imageIDs(previous), imageIDs(current)) {
			return 0, ErrImageGroupBusy
		}
	}

	images, err := i.getImages(ctx, input.ImageIDs)

	if err != nil {
		return 0, err
	}

	members, err := i.loadGroupMembers(ctx, images)

	if err != nil {
		return 0, err
	}

	if err := validateGroupMembers(members, groupId, input.Tag); err != nil {
		return 0, err
	}

	pinned := slices.ContainsFunc(images, func(image model.Image) bool {
		return image.Pinned
	})

	tx, err := i.store.BeginTx(ctx)

	if err != nil {
		return 0, err
	}

	defer tx.Rollback()

	savedID, err := i.writeGroup(ctx, tx, groupId, group, input.Name)

	if err != nil {
		return 0, err
	}

	for _, image := range previous {
		if !slices.Contains(input.ImageIDs, image.ID) {
			if _, err := i.store.UpdateImageGroupMembership(ctx, tx, image.ID, nil); err != nil {
				return 0, err
			}
		}
	}

	for _, member := range members {
		if err := i.writeGroupMember(ctx, tx, member, savedID, input.Tag, pinned); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	i.auditGroupSaved(ctx, group, input, images, pinned)

	return savedID, nil
}

func (i *Images) writeGroup(ctx context.Context, tx store.Transaction, groupId int64, group *model.ImageGroup, name string) (int64, error) {
	var err error

	if group == nil {
		groupId, err = i.store.CreateImageGroup(ctx, tx, name)
	} else if group.Name != name {
		_, err = i.store.UpdateImageGroupName(ctx, tx, groupId, name)
	}

	if errors.Is(err, store.ErrImageGroupNameConflict) {
		return 0, ErrImageGroupAlreadyExists
	}

	return groupId, err
}

func (i *Images) writeGroupMember(ctx context.Context, tx store.Transaction, member imageGroupMember, groupId int64, tag string, pinned bool) error {
	image := member.image

	if image.GroupID == nil {
		if _, err := i.store.UpdateImageGroupMembership(ctx, tx, image.ID, &groupId); err != nil {
			return err
		}
	}

	if image.Tag != tag {
		if err := i.writeTag(ctx, tx, image, tag, model.TagHistorySourceManual); err != nil {
			return err
		}
	}

	if image.Pinned != pinned {
		if _, err := i.store.UpdateImagePinned(ctx, tx, image.ID, pinned); err != nil {
			return fmt.Errorf("updating pinned state for %q: %w", image.Name, err)
		}
	}

	if image.Tag == tag && image.Pinned == pinned {
		return nil
	}

	updateAvailable, updateAvailableTag := effectiveUpdateAvailable(pinned, member.tags, tag)

	return i.writeUpdateAvailable(ctx, tx, &image, updateAvailable, updateAvailableTag)
}

func (i *Images) auditGroupSaved(ctx context.Context, group *model.ImageGroup, input ImageGroupInput, images []model.Image, pinned bool) {
	data := auditImageGroupData{Name: input.Name, Images: imageNames(images), Tag: input.Tag}
	auditType := AuditTypeImageGroupCreated

	if group != nil {
		auditType = AuditTypeImageGroupUpdated

		if group.Name != input.Name {
			data.OldName = group.Name
		}
	}

	recordAudit(ctx, i.audit, auditType, data)

	moved := make([]string, 0, len(images))

	for _, image := range images {
		image.GroupName = &input.Name

		if image.Tag != input.Tag {
			moved = append(moved, image.Name)

			i.auditTagChange(ctx, image, input.Tag)
		}

		if image.Pinned != pinned {
			i.auditPinChange(ctx, image, pinned)
		}
	}

	i.notifyTagChanged(moved...)
}

func (i *Images) DeleteGroup(ctx context.Context, groupId int64) error {
	if groupId < 1 {
		return fmt.Errorf("%w: id must be positive", ErrInvalidImageGroup)
	}

	group, err := i.store.GetImageGroupByID(ctx, groupId)

	if err != nil {
		return err
	}

	if group == nil {
		return fmt.Errorf("%w: %d", ErrImageGroupNotFound, groupId)
	}

	images, err := i.store.ListImageGroupMembers(ctx, groupId)

	if err != nil {
		return err
	}

	unlock, err := i.refreshLocker.lockAll(ctx, imageIDs(images))

	if err != nil {
		return err
	}

	defer unlock()

	deleted, err := i.store.DeleteImageGroup(ctx, groupId)

	if err != nil {
		return err
	}

	if !deleted {
		return fmt.Errorf("%w: %d", ErrImageGroupNotFound, groupId)
	}

	recordAudit(ctx, i.audit, AuditTypeImageGroupDeleted, auditImageGroupData{
		Name:   group.Name,
		Images: imageNames(images),
	})

	return nil
}

func normalizeGroupInput(input ImageGroupInput) (ImageGroupInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Tag = strings.TrimSpace(input.Tag)
	input.ImageIDs = uniqueIDs(input.ImageIDs)

	if input.Name == "" {
		return input, fmt.Errorf("%w: name is required", ErrInvalidImageGroup)
	}

	if len(input.Name) > imageGroupNameMaxLength {
		return input, fmt.Errorf("%w: name must be at most %d characters", ErrInvalidImageGroup, imageGroupNameMaxLength)
	}

	if len(input.ImageIDs) < imageGroupMinImages {
		return input, fmt.Errorf("%w: a group needs at least %d images", ErrInvalidImageGroup, imageGroupMinImages)
	}

	if !tagRE.MatchString(input.Tag) {
		return input, fmt.Errorf("%w: a shared tag is required", ErrInvalidImageGroup)
	}

	return input, nil
}

func validateGroupMembers(members []imageGroupMember, groupId int64, tag string) error {
	var missing, taken []string

	for _, member := range members {
		if member.image.GroupID != nil && *member.image.GroupID != groupId {
			taken = append(taken, member.image.Name)
		}

		if !hasImageTag(member.tags, tag) {
			missing = append(missing, member.image.Name)
		}
	}

	if len(taken) > 0 {
		return fmt.Errorf("%w: %s", ErrImageInAnotherGroup, strings.Join(taken, ", "))
	}

	if len(missing) > 0 {
		return &GroupTagUnavailableError{Tag: tag, Images: missing}
	}

	return nil
}

func uniqueIDs(ids []int64) []int64 {
	unique := make([]int64, 0, len(ids))

	for _, id := range ids {
		if id > 0 && !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}

	return unique
}

func tagPresence(members []imageGroupMember) map[string]int {
	presence := make(map[string]int)

	for _, member := range members {
		for _, tag := range member.tags {
			presence[tag.Tag]++
		}
	}

	return presence
}

func groupLead(members []imageGroupMember, presence map[string]int) (imageGroupMember, string) {
	for _, member := range members {
		for _, candidate := range upgradeCandidates(member.tags, member.image.Tag) {
			if presence[candidate.Tag] == len(members) {
				return member, candidate.Tag
			}
		}
	}

	for _, member := range members {
		if hasImageTag(member.tags, member.image.Tag) {
			return member, ""
		}
	}

	return members[0], ""
}

func commonTags(members []imageGroupMember, lead imageGroupMember, keepTag string) []model.ImageTag {
	presence := tagPresence(members)
	isNew := make(map[string]bool)

	for _, member := range members {
		for _, tag := range member.tags {
			if tag.New {
				isNew[tag.Tag] = true
			}
		}
	}

	common := make([]model.ImageTag, 0)

	for _, tag := range lead.tags {
		if presence[tag.Tag] == len(members) || tag.Tag == keepTag {
			tag.New = isNew[tag.Tag]

			common = append(common, tag)
		}
	}

	return common
}

func applyGroupState(group *model.ImageGroup, members []imageGroupMember) {
	if len(members) == 0 {
		return
	}

	presence := tagPresence(members)

	group.Tag = members[0].image.Tag

	for _, member := range members {
		group.Pinned = group.Pinned || member.image.Pinned
		group.UpdateAvailable = group.UpdateAvailable || member.image.UpdateAvailable
	}

	if !group.UpdateAvailable {
		return
	}

	if _, suggested := groupLead(members, presence); suggested != "" {
		group.UpdateAvailableTag = &suggested
	}

	group.PartialTag = groupPartialTag(members, presence)
}

func groupPartialTag(members []imageGroupMember, presence map[string]int) *model.ImageGroupPartialTag {
	var partial string

	for _, member := range members {
		if member.image.UpdateAvailableTag == nil {
			continue
		}

		tag := *member.image.UpdateAvailableTag

		if presence[tag] < len(members) && presence[tag] > presence[partial] {
			partial = tag
		}
	}

	if partial == "" {
		return nil
	}

	missing := make([]string, 0)

	for _, member := range members {
		if !hasImageTag(member.tags, partial) {
			missing = append(missing, member.image.Name)
		}
	}

	return &model.ImageGroupPartialTag{Tag: partial, Missing: missing}
}
