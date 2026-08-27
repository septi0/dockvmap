package model

import (
	"encoding/json"
	"time"
)

type NotifyLevel string

const (
	NotifyLevelAll      NotifyLevel = "all"
	NotifyLevelUpgrades NotifyLevel = "upgrades"
	NotifyLevelNone     NotifyLevel = "none"
)

func (l NotifyLevel) Valid() bool {
	switch l {
	case NotifyLevelAll, NotifyLevelUpgrades, NotifyLevelNone:
		return true
	default:
		return false
	}
}

type UserPreferences struct {
	NotifyLevel NotifyLevel `json:"notifyLevel"`
}

func (p *UserPreferences) UnmarshalJSON(data []byte) error {
	var raw struct {
		NotifyLevel   *NotifyLevel `json:"notifyLevel"`
		NotifyNewTags *bool        `json:"notifyNewTags"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch {
	case raw.NotifyLevel != nil && raw.NotifyLevel.Valid():
		p.NotifyLevel = *raw.NotifyLevel
	case raw.NotifyNewTags != nil && !*raw.NotifyNewTags:
		p.NotifyLevel = NotifyLevelNone
	default:
		p.NotifyLevel = NotifyLevelAll
	}

	return nil
}

type UserPreferencesUpdate struct {
	NotifyLevel *NotifyLevel
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
