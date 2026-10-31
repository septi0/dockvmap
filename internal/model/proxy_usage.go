package model

import "time"

type ImageUsage struct {
	ImageID     int64
	RequestedAt time.Time
	PulledAt    *time.Time
	PulledTag   string
}

type ProxyTokenUsage struct {
	TokenID int64
	UsedAt  time.Time
}
