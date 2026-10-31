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
	LastRequestedAt    *time.Time `json:"lastRequestedAt,omitempty"`
	LastPulledAt       *time.Time `json:"lastPulledAt,omitempty"`
	LastPulledTag      *string    `json:"lastPulledTag,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
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

func (f ImageStatusFilter) Valid() bool {
	return slices.Contains(ImageStatusFilters, f)
}

type ImageListFilters struct {
	Pagination
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
