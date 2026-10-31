package model

import "time"

type ImageGroup struct {
	ID                 int64
	Name               string
	Images             []Image
	Tag                string
	Pinned             bool
	UpdateAvailable    bool
	UpdateAvailableTag *string
	PartialTag         *ImageGroupPartialTag
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ImageGroupPartialTag struct {
	Tag     string
	Missing []string
}

type ImageGroupListFilters struct {
	Pagination
	Search          string
	UpdateAvailable bool
	Pinned          bool
}
