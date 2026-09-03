package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type listEventsResponse struct {
	Events []model.ImageEvent `json:"events"`
	Total  int64              `json:"total"`
}

const maxEventsLimit = 200

func parseImageEventListFilters(r *http.Request) (model.ImageEventListFilters, error) {
	pagination, err := parsePagination(r, maxEventsLimit)

	if err != nil {
		return model.ImageEventListFilters{}, err
	}

	eventType := r.URL.Query().Get("type")

	if eventType != "" && !service.IsValidTagEventType(eventType) {
		return model.ImageEventListFilters{}, fmt.Errorf("type must be one of the known tag event types")
	}

	var imageID int64

	if value := r.URL.Query().Get("imageId"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)

		if err != nil || parsed < 1 {
			return model.ImageEventListFilters{}, fmt.Errorf("imageId must be a positive integer")
		}

		imageID = parsed
	}

	since, err := parseTimeParam(r, "since")

	if err != nil {
		return model.ImageEventListFilters{}, err
	}

	until, err := parseTimeParam(r, "until")

	if err != nil {
		return model.ImageEventListFilters{}, err
	}

	return model.ImageEventListFilters{
		Pagination: pagination,
		Type:       eventType,
		ImageID:    imageID,
		Since:      since,
		Until:      until,
	}, nil
}

func (w *Web) apiListEvents(rw http.ResponseWriter, r *http.Request) {
	filters, err := parseImageEventListFilters(r)

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())
		return
	}

	events, err := w.events.List(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	total, err := w.events.Count(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusOK, listEventsResponse{
		Events: events,
		Total:  total,
	})
}
