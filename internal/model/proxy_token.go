package model

import "time"

type ProxyToken struct {
	ID        int64
	Label     string
	CreatedAt time.Time
}
