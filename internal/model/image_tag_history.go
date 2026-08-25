package model

import "time"

type TagHistorySource string

const (
	TagHistorySourceCreated TagHistorySource = "created"
	TagHistorySourceManual  TagHistorySource = "manual"
	TagHistorySourceRestore TagHistorySource = "restore"
)

type ImageTagHistory struct {
	ID          int64
	ImageID     int64
	Tag         string
	PreviousTag *string
	Source      TagHistorySource
	AppliedAt   time.Time
}
