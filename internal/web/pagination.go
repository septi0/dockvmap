package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/model"
)

const defaultPageLimit = 25

func parsePagination(r *http.Request, maxLimit int) (model.Pagination, error) {
	limit := defaultPageLimit

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)

		if err != nil || parsed < 1 || parsed > maxLimit {
			return model.Pagination{}, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
		}

		limit = parsed
	}

	offset := 0

	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)

		if err != nil || parsed < 0 {
			return model.Pagination{}, fmt.Errorf("offset must be a non-negative integer")
		}

		offset = parsed
	}

	return model.Pagination{Offset: offset, Limit: limit}, nil
}
