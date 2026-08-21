package web

import (
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/model"
)

type listEventsResponse struct {
	Events     []model.ImageEvent `json:"events"`
	HasMore    bool               `json:"hasMore"`
	NextOffset int                `json:"nextOffset"`
}

func (w *Web) apiListEvents(rw http.ResponseWriter, r *http.Request) {
	limit := 25
	offset := 0

	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)

		if err != nil || parsed < 0 {
			apiError(rw, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}

		offset = parsed
	}

	events, err := w.events.List(r.Context(), offset, limit+1)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	hasMore := len(events) > limit

	if hasMore {
		events = events[:limit]
	}

	apiJSON(rw, http.StatusOK, listEventsResponse{
		Events:     events,
		HasMore:    hasMore,
		NextOffset: offset + len(events),
	})
}
