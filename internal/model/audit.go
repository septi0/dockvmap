package model

import (
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	IP        string          `json:"ip,omitempty"`
	UserAgent string          `json:"userAgent,omitempty"`
	UserID    int64           `json:"userId,omitempty"`
	Username  string          `json:"username,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

type AuditLogListFilters struct {
	Pagination
	Type  string
	Since *time.Time
	Until *time.Time
}
