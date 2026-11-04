package model

import (
	"slices"
	"time"
)

const (
	RefreshStatusIdle    = "idle"
	RefreshStatusRunning = "running"
)

type Image struct {
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
	GroupID            *int64     `json:"groupId,omitempty"`
	GroupName          *string    `json:"groupName,omitempty"`
	TagSetHash         string     `json:"-"`
	LastPulledAt       *time.Time `json:"lastPulledAt,omitempty"`
	LastPulledTag      *string    `json:"lastPulledTag,omitempty"`
	CurrentDigest      *string    `json:"currentDigest,omitempty"`
	CurrentDigestTag   *string    `json:"currentDigestTag,omitempty"`
	CurrentDigestSince *time.Time `json:"currentDigestSince,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (i Image) TagDigest() (*string, *time.Time) {
	if i.CurrentDigestTag == nil || *i.CurrentDigestTag != i.Tag {
		return nil, nil
	}

	return i.CurrentDigest, i.CurrentDigestSince
}

type ImageStatusFilter string

const (
	ImageStatusUpdateAvailable ImageStatusFilter = "updateAvailable"
	ImageStatusFailedCheck     ImageStatusFilter = "failedCheck"
	ImageStatusPinned          ImageStatusFilter = "pinned"
)

var ImageStatusFilters = []ImageStatusFilter{
	ImageStatusUpdateAvailable,
	ImageStatusFailedCheck,
	ImageStatusPinned,
}

const (
	ImageSortName        = "name"
	ImageSortLastChecked = "lastChecked"
	ImageSortLastPulled  = "lastPulled"
	ImageSortStatus      = "status"
)

var ImageSortFields = []string{ImageSortName, ImageSortLastChecked, ImageSortLastPulled, ImageSortStatus}

func (f ImageStatusFilter) Valid() bool {
	return slices.Contains(ImageStatusFilters, f)
}

type ImageListFilters struct {
	Pagination
	Sort
	Search    string
	Status    ImageStatusFilter
	Ungrouped bool
	GroupID   int64
}

type ImageStatusCounts struct {
	Total           int64
	UpdateAvailable int64
	FailedCheck     int64
}
