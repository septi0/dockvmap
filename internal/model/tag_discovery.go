package model

import "time"

type TagDiscoveryStatus string

const (
	TagDiscoveryRunning   TagDiscoveryStatus = "running"
	TagDiscoveryCompleted TagDiscoveryStatus = "completed"
	TagDiscoveryFailed    TagDiscoveryStatus = "failed"
)

type TagDiscoveryTag struct {
	Tag        string `json:"tag"`
	Prerelease bool   `json:"prerelease"`
}

type TagDiscoveryGroup struct {
	FamilyID   int64             `json:"familyId"`
	FamilyType string            `json:"familyType"`
	HasOrder   bool              `json:"hasOrder"`
	Tags       []TagDiscoveryTag `json:"tags"`
}

type TagDiscovery struct {
	ID          int64
	RegistryID  int64
	Repository  string
	Status      TagDiscoveryStatus
	TagGroups   []TagDiscoveryGroup
	TagCount    int
	RawTagCount int
	Error       string
	StartedAt   time.Time
	CompletedAt *time.Time
	// TagsSeen is in-memory only while Status is running; never persisted.
	TagsSeen int
}
