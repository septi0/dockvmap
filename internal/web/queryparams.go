package web

import (
	"fmt"
	"net/http"
	"strconv"
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

func parseBoolParam(r *http.Request, name string) (*bool, error) {
	value := r.URL.Query().Get(name)

	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseBool(value)

	if err != nil {
		return nil, fmt.Errorf("%s must be a boolean", name)
	}

	return &parsed, nil
}
