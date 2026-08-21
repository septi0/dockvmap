package model

import "time"

type CurrentUser struct {
	ID       int64
	Username string
}

type Session struct {
	ID        int64
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
	Current   bool
}
