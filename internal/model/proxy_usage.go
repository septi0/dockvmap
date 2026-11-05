package model

import "time"

type ImageUsage struct {
	ImageID   int64
	PulledAt  time.Time
	PulledTag string
}

type ProxyTokenUsage struct {
	TokenID int64
	UsedAt  time.Time
}

type ImageDigest struct {
	ImageID int64
	Tag     string
	Digest  string
	SeenAt  time.Time
}
