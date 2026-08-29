package model

import "time"

type CachedBlob struct {
	Digest      string
	Size        int64
	ContentType string
	CreatedAt   time.Time
	AccessedAt  time.Time
}
