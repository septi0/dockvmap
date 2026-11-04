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
	Internal                 bool                  `json:"internal"`
}

func newRegistryResponse(info model.RegistryInfo) registryResponse {
	return registryResponse{
		ID:                       info.ID,
		Registry:                 info.Registry,
		Username:                 info.Username,
		AuthenticationConfigured: info.AuthenticationConfigured,
		Options:                  info.Options,
		Internal:                 info.Internal,
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
	RegistryInternal   bool       `json:"registryInternal"`
	Repository         string     `json:"repository"`
	Tag                string     `json:"tag"`
	LastChecked        *time.Time `json:"lastChecked,omitempty"`
	LastCheckError     *string    `json:"lastCheckError,omitempty"`
	UpdateAvailable    bool       `json:"updateAvailable"`
	UpdateAvailableTag *string    `json:"updateAvailableTag,omitempty"`
	Pinned             bool       `json:"pinned"`
	RefreshStatus      string     `json:"refreshStatus"`
	Group              *imageRef  `json:"group,omitempty"`
	LastPulledAt       *time.Time `json:"lastPulledAt,omitempty"`
	LastPulledTag      *string    `json:"lastPulledTag,omitempty"`
	CurrentDigest      *string    `json:"currentDigest,omitempty"`
	CurrentDigestSince *time.Time `json:"currentDigestSince,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type imageRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func newImageResponse(image model.Image) imageResponse {
	var group *imageRef

	if image.GroupID != nil && image.GroupName != nil {
		group = &imageRef{ID: *image.GroupID, Name: *image.GroupName}
	}

	digest, digestSince := image.TagDigest()

	return imageResponse{
		ID:                 image.ID,
		Name:               image.Name,
		RegistryID:         image.RegistryID,
		Registry:           image.Registry,
		RegistryInternal:   image.RegistryInternal,
		Repository:         image.Repository,
		Tag:                image.Tag,
		LastChecked:        image.LastChecked,
		LastCheckError:     image.LastCheckError,
		UpdateAvailable:    image.UpdateAvailable,
		UpdateAvailableTag: image.UpdateAvailableTag,
		Pinned:             image.Pinned,
		RefreshStatus:      image.RefreshStatus,
		Group:              group,
		LastPulledAt:       image.LastPulledAt,
		LastPulledTag:      image.LastPulledTag,
		CurrentDigest:      digest,
		CurrentDigestSince: digestSince,
		CreatedAt:          image.CreatedAt,
		UpdatedAt:          image.UpdatedAt,
	}
}

type imageGroupRequest struct {
	Name     string  `json:"name"`
	ImageIDs []int64 `json:"imageIds"`
	Tag      string  `json:"tag"`
}

type imageGroupPartialTagResponse struct {
	Tag     string   `json:"tag"`
	Missing []string `json:"missing"`
}

type imageGroupResponse struct {
	ID                 int64                         `json:"id"`
	Name               string                        `json:"name"`
	Tag                string                        `json:"tag"`
	Pinned             bool                          `json:"pinned"`
	UpdateAvailable    bool                          `json:"updateAvailable"`
	UpdateAvailableTag *string                       `json:"updateAvailableTag,omitempty"`
	PartialTag         *imageGroupPartialTagResponse `json:"partialTag,omitempty"`
	Images             []imageResponse               `json:"images"`
	CreatedAt          time.Time                     `json:"createdAt"`
	UpdatedAt          time.Time                     `json:"updatedAt"`
}

func newImageGroupResponse(group model.ImageGroup) imageGroupResponse {
	images := make([]imageResponse, 0, len(group.Images))

	for _, image := range group.Images {
		images = append(images, newImageResponse(image))
	}

	var partial *imageGroupPartialTagResponse

	if group.PartialTag != nil {
		partial = &imageGroupPartialTagResponse{Tag: group.PartialTag.Tag, Missing: group.PartialTag.Missing}
	}

	return imageGroupResponse{
		ID:                 group.ID,
		Name:               group.Name,
		Tag:                group.Tag,
		Pinned:             group.Pinned,
		UpdateAvailable:    group.UpdateAvailable,
		UpdateAvailableTag: group.UpdateAvailableTag,
		PartialTag:         partial,
		Images:             images,
		CreatedAt:          group.CreatedAt,
		UpdatedAt:          group.UpdatedAt,
	}
}

func newImageGroupResponses(groups []model.ImageGroup) []imageGroupResponse {
	responses := make([]imageGroupResponse, 0, len(groups))

	for _, group := range groups {
		responses = append(responses, newImageGroupResponse(group))
	}

	return responses
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

type setImagePinRequest struct {
	Pinned bool `json:"pinned"`
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
	Label   string `json:"label"`
	CanPush bool   `json:"canPush"`
}

type proxyTokenResponse struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	CanPush    bool       `json:"canPush"`
}

func newProxyTokenResponse(token model.ProxyToken) proxyTokenResponse {
	return proxyTokenResponse{
		ID:         token.ID,
		Label:      token.Label,
		CreatedAt:  token.CreatedAt,
		LastUsedAt: token.LastUsedAt,
		CanPush:    token.CanPush,
	}
}

type createProxyTokenResponse struct {
	ID      int64  `json:"id"`
	Label   string `json:"label"`
	CanPush bool   `json:"canPush"`
	Token   string `json:"token"`
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
	OccurredAt  time.Time `json:"occurredAt"`
	Source      string    `json:"source"`
	Message     string    `json:"message"`
	Occurrences int       `json:"occurrences"`
}

func newFailureResponse(failure service.Failure) failureResponse {
	return failureResponse{
		OccurredAt:  failure.OccurredAt,
		Source:      string(failure.Source),
		Message:     failureMessage(failure),
		Occurrences: failure.Occurrences,
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

	case service.FailureSourceDigest:
		return fmt.Sprintf("Image %q digest lookup failed: %s", failure.Detail, failure.Error)

	case service.FailureSourceInternalRegistry:
		return fmt.Sprintf("Internal registry %s %s", failure.Detail, failure.Error)

	default:
		return failure.Error
	}
}

type hostResponse struct {
	ID                  int64            `json:"id"`
	Name                string           `json:"name"`
	Address             string           `json:"address"`
	AgentFingerprint    *string          `json:"agentFingerprint,omitempty"`
	CreatedAt           time.Time        `json:"createdAt"`
	LastSeenAt          *time.Time       `json:"lastSeenAt,omitempty"`
	PollError           *string          `json:"pollError,omitempty"`
	PollErrorKind       *string          `json:"pollErrorKind,omitempty"`
	Online              bool             `json:"online"`
	Current             bool             `json:"current"`
	AgentVersion        *string          `json:"agentVersion,omitempty"`
	AgentUpdateNeeded   bool             `json:"agentUpdateNeeded"`
	DockerError         *string          `json:"dockerError,omitempty"`
	Info                hostInfoResponse `json:"info"`
	RunningContainers   int              `json:"runningContainers"`
	StoppedContainers   int              `json:"stoppedContainers"`
	Stacks              int              `json:"stacks"`
	UnhealthyContainers int              `json:"unhealthyContainers"`
	CrashedContainers   int              `json:"crashedContainers"`
	ProxyContainers     int              `json:"proxyContainers"`
	Rollout             rolloutResponse  `json:"rollout"`
	LastStartedAt       *time.Time       `json:"lastStartedAt,omitempty"`
}

type rolloutResponse struct {
	UpToDate int `json:"upToDate"`
	Total    int `json:"total"`
}

type hostInfoResponse struct {
	Hostname      *string `json:"hostname,omitempty"`
	DockerVersion *string `json:"dockerVersion,omitempty"`
	OS            *string `json:"os,omitempty"`
	Architecture  *string `json:"architecture,omitempty"`
	CPUs          *int    `json:"cpus,omitempty"`
	MemoryBytes   *int64  `json:"memoryBytes,omitempty"`
}

type hostContainerResponse struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Image           string                     `json:"image"`
	State           string                     `json:"state"`
	Health          *string                    `json:"health,omitempty"`
	ExitCode        int                        `json:"exitCode"`
	Crashed         bool                       `json:"crashed"`
	RecentlyCrashed bool                       `json:"recentlyCrashed"`
	ComposeProject  *string                    `json:"composeProject,omitempty"`
	ComposeService  *string                    `json:"composeService,omitempty"`
	CreatedAt       *time.Time                 `json:"createdAt,omitempty"`
	StartedAt       *time.Time                 `json:"startedAt,omitempty"`
	FinishedAt      *time.Time                 `json:"finishedAt,omitempty"`
	Ports           []model.ContainerPort      `json:"ports"`
	Tracking        *containerTrackingResponse `json:"tracking,omitempty"`
}

type containerTrackingResponse struct {
	ImageID     *int64  `json:"imageId,omitempty"`
	ImageName   string  `json:"imageName"`
	Status      string  `json:"status"`
	PinnedTag   *string `json:"pinnedTag,omitempty"`
	NeedsUpdate bool    `json:"needsUpdate"`
}

type hostDetailsResponse struct {
	Host       hostResponse            `json:"host"`
	Containers []hostContainerResponse `json:"containers"`
}

type imageRolloutResponse struct {
	ImageID int64 `json:"imageId"`
	rolloutResponse
	HostsOffline     int `json:"hostsOffline"`
	HostsDockerError int `json:"hostsDockerError"`
}

type imageContainersResponse struct {
	Hosts           []hostDetailsResponse `json:"hosts"`
	Rollout         rolloutResponse       `json:"rollout"`
	ConfiguredHosts int                   `json:"configuredHosts"`
}

type hostRequest struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func newHostResponse(host model.Host) hostResponse {
	return hostResponse{
		ID:                host.ID,
		Name:              host.Name,
		Address:           host.Address,
		AgentFingerprint:  host.AgentFingerprint,
		CreatedAt:         host.CreatedAt,
		LastSeenAt:        host.LastSeenAt,
		PollError:         host.PollError,
		PollErrorKind:     host.PollErrorKind,
		Online:            host.Online,
		Current:           host.Current(),
		AgentVersion:      host.AgentVersion,
		AgentUpdateNeeded: host.AgentUpdateNeeded,
		DockerError:       host.DockerError,
		Info: hostInfoResponse{
			Hostname:      host.Info.Hostname,
			DockerVersion: host.Info.DockerVersion,
			OS:            host.Info.OS,
			Architecture:  host.Info.Architecture,
			CPUs:          host.Info.CPUs,
			MemoryBytes:   host.Info.MemoryBytes,
		},
		RunningContainers:   host.RunningContainers,
		StoppedContainers:   host.StoppedContainers,
		Stacks:              host.Stacks,
		UnhealthyContainers: host.UnhealthyContainers,
		CrashedContainers:   host.CrashedContainers,
		ProxyContainers:     host.ProxyContainers,
		Rollout:             newRolloutResponse(host.Rollout),
		LastStartedAt:       host.LastStartedAt,
	}
}

func newRolloutResponse(rollout model.Rollout) rolloutResponse {
	return rolloutResponse{UpToDate: rollout.UpToDate, Total: rollout.Total}
}

func newHostContainerResponses(containers []model.HostContainer) []hostContainerResponse {
	responses := make([]hostContainerResponse, 0, len(containers))

	for _, c := range containers {
		response := hostContainerResponse{
			ID:              c.ContainerID,
			Name:            c.Name,
			Image:           c.Image,
			State:           c.State,
			Health:          c.Health,
			ExitCode:        c.ExitCode,
			Crashed:         c.Crashed(),
			RecentlyCrashed: c.RecentlyCrashed,
			ComposeProject:  c.ComposeProject,
			ComposeService:  c.ComposeService,
			CreatedAt:       c.CreatedAt,
			StartedAt:       c.StartedAt,
			FinishedAt:      c.FinishedAt,
			Ports:           c.Ports,
		}

		if response.Ports == nil {
			response.Ports = []model.ContainerPort{}
		}

		if c.Tracking != nil {
			response.Tracking = &containerTrackingResponse{
				ImageID:     c.Tracking.ImageID,
				ImageName:   c.Tracking.ImageName,
				Status:      string(c.Tracking.Status),
				PinnedTag:   c.Tracking.PinnedTag,
				NeedsUpdate: c.Tracking.NeedsUpdate(),
			}
		}

		responses = append(responses, response)
	}

	return responses
}
