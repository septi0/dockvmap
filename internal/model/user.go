package model

import "time"

type UserPreferences struct {
	NotifyNewTags bool `json:"notifyNewTags"`
}

type UserPreferencesUpdate struct {
	NotifyNewTags *bool
}

type User struct {
	ID           int64           `json:"id"`
	Username     string          `json:"username"`
	Email        string          `json:"email"`
	PasswordHash string          `json:"-"`
	Preferences  UserPreferences `json:"preferences"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}
