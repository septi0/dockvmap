package model

import "time"

// ImageUsage and ProxyTokenUsage are also what followers report to their primary (replicationapi.PullReport)
type ImageUsage struct {
	ImageID   int64     `json:"imageId"`
	PulledAt  time.Time `json:"pulledAt"`
	PulledTag string    `json:"pulledTag"`
}

type ProxyTokenUsage struct {
	TokenID int64     `json:"tokenId"`
	UsedAt  time.Time `json:"usedAt"`
}
