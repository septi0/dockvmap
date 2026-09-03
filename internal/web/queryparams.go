package web

import (
	"fmt"
	"net/http"
	"time"
)

func parseTimeParam(r *http.Request, name string) (*time.Time, error) {
	value := r.URL.Query().Get(name)

	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)

	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp", name)
	}

	return &parsed, nil
}

func parseDateRange(r *http.Request) (since, until *time.Time, err error) {
	if since, err = parseTimeParam(r, "since"); err != nil {
		return nil, nil, err
	}

	if until, err = parseTimeParam(r, "until"); err != nil {
		return nil, nil, err
	}

	return since, until, nil
}
