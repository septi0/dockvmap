package web

import (
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/taganalyzer"
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
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	RegistryID      int64      `json:"registryId"`
	Registry        string     `json:"registry"`
	Repository      string     `json:"repository"`
	Tag             string     `json:"tag"`
	LastChecked     *time.Time `json:"lastChecked,omitempty"`
	LastCheckError  *string    `json:"lastCheckError,omitempty"`
	UpdateAvailable bool       `json:"updateAvailable"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func newImageResponse(image model.Image) imageResponse {
	return imageResponse{
		ID:              image.ID,
		Name:            image.Name,
		RegistryID:      image.RegistryID,
		Registry:        image.Registry,
		Repository:      image.Repository,
		Tag:             image.Tag,
		LastChecked:     image.LastChecked,
		LastCheckError:  image.LastCheckError,
		UpdateAvailable: image.UpdateAvailable,
		CreatedAt:       image.CreatedAt,
		UpdatedAt:       image.UpdatedAt,
	}
}

type updateImageTagRequest struct {
	Tag string `json:"tag"`
}

type renameImageRequest struct {
	Name string `json:"name"`
}

type pullInfoResponse struct {
	Host       string `json:"host"`
	Port       string `json:"port"`
	VirtualTag string `json:"virtualTag"`
}

type inspectRepositoryRequest struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}

type tagResponse struct {
	Tag       string     `json:"tag"`
	FirstSeen *time.Time `json:"firstSeen,omitempty"`
	LastSeen  *time.Time `json:"lastSeen,omitempty"`
	New       bool       `json:"new,omitempty"`
}

type tagGroupResponse struct {
	FamilyType string        `json:"familyType"`
	FamilyID   int64         `json:"familyId"`
	Tags       []tagResponse `json:"tags"`
}

type inspectRepositoryResponse struct {
	Registry   string             `json:"registry"`
	Repository string             `json:"repository"`
	TagGroups  []tagGroupResponse `json:"tagGroups"`
}

func newInspectRepositoryResponse(registry string, repository string, analysis taganalyzer.Analysis) inspectRepositoryResponse {
	tagGroups := make([]tagGroupResponse, 0, len(analysis.Ordered))

	for _, family := range analysis.Ordered {
		tags := make([]tagResponse, 0, len(family.OrderedTags))

		for _, tag := range family.OrderedTags {
			tags = append(tags, tagResponse{
				Tag: tag,
			})
		}

		tagGroups = append(tagGroups, tagGroupResponse{
			FamilyType: string(family.Kind),
			FamilyID:   int64(family.ID),
			Tags:       tags,
		})
	}

	return inspectRepositoryResponse{
		Registry:   registry,
		Repository: repository,
		TagGroups:  tagGroups,
	}
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
				Tags:       make([]tagResponse, 0),
			})

			group = &tagGroups[len(tagGroups)-1]
		}

		group.Tags = append(group.Tags, tagResponse{
			Tag:       tag.Tag,
			FirstSeen: &tag.FirstSeen,
			LastSeen:  &tag.LastSeen,
			New:       tag.New,
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
	NotifyNewTags *bool `json:"notifyNewTags,omitempty"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type currentUserResponse struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	Email         string `json:"email"`
	NotifyNewTags bool   `json:"notifyNewTags"`
}

func newCurrentUserResponse(user model.User) currentUserResponse {
	return currentUserResponse{
		ID:            user.ID,
		Username:      user.Username,
		Email:         user.Email,
		NotifyNewTags: user.Preferences.NotifyNewTags,
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

type proxyMetricsResponse struct {
	StartedAt          time.Time `json:"startedAt"`
	CacheEnabled       bool      `json:"cacheEnabled"`
	TotalRequests      uint64    `json:"totalRequests"`
	ManifestRequests   uint64    `json:"manifestRequests"`
	BlobRequests       uint64    `json:"blobRequests"`
	CacheHits          uint64    `json:"cacheHits"`
	CacheMisses        uint64    `json:"cacheMisses"`
	UpstreamRequests   uint64    `json:"upstreamRequests"`
	UpstreamFailures   uint64    `json:"upstreamFailures"`
	CacheWriteFailures uint64    `json:"cacheWriteFailures"`
}

func newProxyMetricsResponse(snapshot proxy.MetricsSnapshot, cacheEnabled bool) proxyMetricsResponse {
	return proxyMetricsResponse{
		StartedAt:          snapshot.StartedAt,
		CacheEnabled:       cacheEnabled,
		TotalRequests:      snapshot.TotalRequests,
		ManifestRequests:   snapshot.ManifestRequests,
		BlobRequests:       snapshot.BlobRequests,
		CacheHits:          snapshot.CacheHits,
		CacheMisses:        snapshot.CacheMisses,
		UpstreamRequests:   snapshot.UpstreamRequests,
		UpstreamFailures:   snapshot.UpstreamFailures,
		CacheWriteFailures: snapshot.CacheWriteFailures,
	}
}

type recentFailureResponse struct {
	OccurredAt time.Time `json:"occurredAt"`
	Message    string    `json:"message"`
}

func newRecentFailureResponse(failure service.Failure) recentFailureResponse {
	return recentFailureResponse{
		OccurredAt: failure.OccurredAt,
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

	default:
		return failure.Error
	}
}
