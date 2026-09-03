package web

import (
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type createRegistryRequest struct {
	Registry   string                `json:"registry"`
	Username   string                `json:"username"`
	Credential string                `json:"credential"`
	Options    model.RegistryOptions `json:"options"`
}

type updateRegistryRequest struct {
	Registry   string                 `json:"registry"`
	Username   *string                `json:"username"`
	Credential *string                `json:"credential"`
	Options    *model.RegistryOptions `json:"options"`
}

type registryResponse struct {
	ID                       int64                 `json:"id"`
	Registry                 string                `json:"registry"`
	Username                 string                `json:"username,omitempty"`
	AuthenticationConfigured bool                  `json:"authenticationConfigured"`
	Options                  model.RegistryOptions `json:"options"`
}

func newRegistryResponse(info model.RegistryInfo) registryResponse {
	return registryResponse{
		ID:                       info.ID,
		Registry:                 info.Registry,
		Username:                 info.Username,
		AuthenticationConfigured: info.AuthenticationConfigured,
		Options:                  info.Options,
	}
}

type createImageRequest struct {
	Name       string `json:"name"`
	RegistryID int64  `json:"registryId"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

type imageResponse struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	RegistryID         int64      `json:"registryId"`
	Registry           string     `json:"registry"`
	Repository         string     `json:"repository"`
	Tag                string     `json:"tag"`
	LastChecked        *time.Time `json:"lastChecked,omitempty"`
	LastCheckError     *string    `json:"lastCheckError,omitempty"`
	UpdateAvailable    bool       `json:"updateAvailable"`
	UpdateAvailableTag *string    `json:"updateAvailableTag,omitempty"`
	RefreshStatus      string     `json:"refreshStatus"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func newImageResponse(image model.Image) imageResponse {
	return imageResponse{
		ID:                 image.ID,
		Name:               image.Name,
		RegistryID:         image.RegistryID,
		Registry:           image.Registry,
		Repository:         image.Repository,
		Tag:                image.Tag,
		LastChecked:        image.LastChecked,
		LastCheckError:     image.LastCheckError,
		UpdateAvailable:    image.UpdateAvailable,
		UpdateAvailableTag: image.UpdateAvailableTag,
		RefreshStatus:      image.RefreshStatus,
		CreatedAt:          image.CreatedAt,
		UpdatedAt:          image.UpdatedAt,
	}
}

type updateImageTagRequest struct {
	Tag    string `json:"tag"`
	Source string `json:"source,omitempty"`
}

type tagHistoryEntryResponse struct {
	ID          int64     `json:"id"`
	Tag         string    `json:"tag"`
	PreviousTag *string   `json:"previousTag,omitempty"`
	Source      string    `json:"source"`
	AppliedAt   time.Time `json:"appliedAt"`
}

type tagHistoryResponse struct {
	History []tagHistoryEntryResponse `json:"history"`
}

func newTagHistoryResponse(history []model.ImageTagHistory) tagHistoryResponse {
	entries := make([]tagHistoryEntryResponse, 0, len(history))

	for _, h := range history {
		entries = append(entries, tagHistoryEntryResponse{
			ID:          h.ID,
			Tag:         h.Tag,
			PreviousTag: h.PreviousTag,
			Source:      string(h.Source),
			AppliedAt:   h.AppliedAt,
		})
	}

	return tagHistoryResponse{History: entries}
}

type renameImageRequest struct {
	Name string `json:"name"`
}

type pullInfoResponse struct {
	Host           string `json:"host"`
	Port           string `json:"port"`
	VirtualTag     string `json:"virtualTag"`
	HostConfigured bool   `json:"hostConfigured"`
}

type inspectRepositoryRequest struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}

type tagResponse struct {
	Tag        string     `json:"tag"`
	FirstSeen  *time.Time `json:"firstSeen,omitempty"`
	LastSeen   *time.Time `json:"lastSeen,omitempty"`
	New        bool       `json:"new,omitempty"`
	Prerelease bool       `json:"prerelease,omitempty"`
}

type tagGroupResponse struct {
	FamilyType string        `json:"familyType"`
	FamilyID   int64         `json:"familyId"`
	HasOrder   bool          `json:"hasOrder"`
	Tags       []tagResponse `json:"tags"`
}

type discoveryResponse struct {
	ID           int64              `json:"id"`
	Status       string             `json:"status"`
	TagGroups    []tagGroupResponse `json:"tagGroups,omitempty"`
	TagCount     int                `json:"tagCount,omitempty"`
	IgnoredCount int                `json:"ignoredCount,omitempty"`
	TagsSeen     int                `json:"tagsSeen,omitempty"`
	Error        string             `json:"error,omitempty"`
}

func newDiscoveryResponse(discovery model.TagDiscovery) discoveryResponse {
	resp := discoveryResponse{
		ID:     discovery.ID,
		Status: string(discovery.Status),
		Error:  discovery.Error,
	}

	if discovery.Status == model.TagDiscoveryCompleted {
		resp.TagGroups = tagGroupResponsesFromDiscoveryGroups(discovery.TagGroups)
		resp.TagCount = discovery.TagCount
		resp.IgnoredCount = discovery.RawTagCount - discovery.TagCount
	}

	if discovery.Status == model.TagDiscoveryRunning {
		resp.TagsSeen = discovery.TagsSeen
	}

	return resp
}

func tagGroupResponsesFromDiscoveryGroups(groups []model.TagDiscoveryGroup) []tagGroupResponse {
	responses := make([]tagGroupResponse, 0, len(groups))

	for _, group := range groups {
		tags := make([]tagResponse, 0, len(group.Tags))

		for _, tag := range group.Tags {
			tags = append(tags, tagResponse{
				Tag:        tag.Tag,
				Prerelease: tag.Prerelease,
			})
		}

		responses = append(responses, tagGroupResponse{
			FamilyType: group.FamilyType,
			FamilyID:   group.FamilyID,
			HasOrder:   group.HasOrder,
			Tags:       tags,
		})
	}

	return responses
}

type imageTagsResponse struct {
	TagGroups []tagGroupResponse `json:"tagGroups"`
}

func newImageTagsResponse(tags []model.ImageTag, currentTag string) imageTagsResponse {
	tagGroups := make([]tagGroupResponse, 0)

	for _, tag := range tags {
		var group *tagGroupResponse

		for i := range tagGroups {
			if tagGroups[i].FamilyID == tag.FamilyID {
				group = &tagGroups[i]

				break
			}
		}

		if group == nil {
			tagGroups = append(tagGroups, tagGroupResponse{
				FamilyType: tag.FamilyType,
				FamilyID:   tag.FamilyID,
				HasOrder:   tag.FamilyHasOrder,
				Tags:       make([]tagResponse, 0),
			})

			group = &tagGroups[len(tagGroups)-1]
		}

		group.Tags = append(group.Tags, tagResponse{
			Tag:        tag.Tag,
			FirstSeen:  &tag.FirstSeen,
			LastSeen:   &tag.LastSeen,
			New:        tag.New,
			Prerelease: tag.Prerelease,
		})
	}

	currentGroupIndex := -1

	for i, group := range tagGroups {
		for _, t := range group.Tags {
			if t.Tag == currentTag {
				currentGroupIndex = i

				break
			}
		}

		if currentGroupIndex != -1 {
			break
		}
	}

	if currentGroupIndex > 0 {
		currentGroup := tagGroups[currentGroupIndex]
		tagGroups = append(tagGroups[:currentGroupIndex], tagGroups[currentGroupIndex+1:]...)
		tagGroups = append([]tagGroupResponse{currentGroup}, tagGroups...)
	}

	return imageTagsResponse{
		TagGroups: tagGroups,
	}
}

type bootstrapUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateUserPasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type updateUserEmailRequest struct {
	Email string `json:"email"`
}

type updateUserPreferencesRequest struct {
	NotifyLevel *string `json:"notifyLevel,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type currentUserResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	NotifyLevel string `json:"notifyLevel"`
}

func newCurrentUserResponse(user model.User) currentUserResponse {
	return currentUserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		NotifyLevel: string(user.Preferences.NotifyLevel),
	}
}

type sessionResponse struct {
	ID        int64     `json:"id"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

func newSessionResponse(session model.Session) sessionResponse {
	return sessionResponse{
		ID:        session.ID,
		IP:        session.IP,
		UserAgent: session.UserAgent,
		CreatedAt: session.CreatedAt,
		ExpiresAt: session.ExpiresAt,
		Current:   session.Current,
	}
}

type createProxyTokenRequest struct {
	Label string `json:"label"`
}

type proxyTokenResponse struct {
	ID        int64     `json:"id"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
}

func newProxyTokenResponse(token model.ProxyToken) proxyTokenResponse {
	return proxyTokenResponse{
		ID:        token.ID,
		Label:     token.Label,
		CreatedAt: token.CreatedAt,
	}
}

type createProxyTokenResponse struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Token string `json:"token"`
}

type proxyCacheUsageResponse struct {
	UsedBytes int64 `json:"usedBytes"`
	MaxBytes  int64 `json:"maxBytes"`
}

type proxyMetricsResponse struct {
	GeneratedAt time.Time                 `json:"generatedAt"`
	Windows     model.ProxyMetricsSummary `json:"windows"`
	Cache       *proxyCacheUsageResponse  `json:"cache"`
}

type tagRefreshStatusResponse struct {
	Enabled  bool       `json:"enabled"`
	Interval string     `json:"interval"`
	Running  bool       `json:"running"`
	LastRun  *time.Time `json:"lastRun"`
	NextDue  *time.Time `json:"nextDue"`
}

func newTagRefreshStatusResponse(enabled bool, interval string, running bool, lastRun, nextDue *time.Time) tagRefreshStatusResponse {
	return tagRefreshStatusResponse{
		Enabled:  enabled,
		Interval: interval,
		Running:  running,
		LastRun:  lastRun,
		NextDue:  nextDue,
	}
}

type failureResponse struct {
	OccurredAt time.Time `json:"occurredAt"`
	Source     string    `json:"source"`
	Message    string    `json:"message"`
}

func newFailureResponse(failure service.Failure) failureResponse {
	return failureResponse{
		OccurredAt: failure.OccurredAt,
		Source:     string(failure.Source),
		Message:    failureMessage(failure),
	}
}

func failureMessage(failure service.Failure) string {
	switch failure.Source {
	case service.FailureSourceWebhook:
		return fmt.Sprintf("Webhook to %s failed: %s", failure.Detail, failure.Error)

	case service.FailureSourceEmail:
		return fmt.Sprintf("Email notification failed: %s", failure.Error)

	case service.FailureSourceRefresh:
		return fmt.Sprintf("Image %q failed to refresh: %s", failure.Detail, failure.Error)

	case service.FailureSourceDiscovery:
		return fmt.Sprintf("Repository %q tag discovery failed: %s", failure.Detail, failure.Error)

	case service.FailureSourceEventRegistration:
		return fmt.Sprintf("Image %q refreshed but its tag-change notification failed to register: %s", failure.Detail, failure.Error)

	default:
		return failure.Error
	}
}
