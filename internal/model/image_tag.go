package model

import "time"

type ImageTag struct {
	ID              int64     `json:"id"`
	ImageID         int64     `json:"imageId"`
	FamilyID        int64     `json:"familyId"`
	FamilyType      string    `json:"familyType"`
	FamilyHasOrder  bool      `json:"familyHasOrder"`
	Tag             string    `json:"tag"`
	TagOrder        int       `json:"tag_order"`
	Prerelease      bool      `json:"prerelease"`
	VersionSegments [][]int64 `json:"-"`
	FirstSeen       time.Time `json:"firstSeen"`
	LastSeen        time.Time `json:"lastSeen"`
	New             bool      `json:"new"`
}
