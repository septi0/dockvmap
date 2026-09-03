package model

import "time"

type BackgroundFailure struct {
	ID         int64
	Source     string
	Detail     string
	Error      string
	OccurredAt time.Time
}

type BackgroundFailureListFilters struct {
	Pagination
	Source string
	Since  *time.Time
	Until  *time.Time
}
