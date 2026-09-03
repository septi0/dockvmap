package web

import (
	"fmt"
	"net/http"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type listFailuresResponse struct {
	Failures []failureResponse `json:"failures"`
	Total    int64             `json:"total"`
}

const maxFailuresLimit = 200

func parseFailureListFilters(r *http.Request) (model.BackgroundFailureListFilters, error) {
	pagination, err := parsePagination(r, maxFailuresLimit)

	if err != nil {
		return model.BackgroundFailureListFilters{}, err
	}

	source := r.URL.Query().Get("source")

	if source != "" && !service.IsValidFailureSource(source) {
		return model.BackgroundFailureListFilters{}, fmt.Errorf("source must be one of the known failure sources")
	}

	since, err := parseTimeParam(r, "since")

	if err != nil {
		return model.BackgroundFailureListFilters{}, err
	}

	until, err := parseTimeParam(r, "until")

	if err != nil {
		return model.BackgroundFailureListFilters{}, err
	}

	return model.BackgroundFailureListFilters{
		Pagination: pagination,
		Source:     source,
		Since:      since,
		Until:      until,
	}, nil
}

func (w *Web) apiListFailures(rw http.ResponseWriter, r *http.Request) {
	filters, err := parseFailureListFilters(r)

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())
		return
	}

	failures, err := w.failures.List(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	total, err := w.failures.Count(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]failureResponse, 0, len(failures))

	for _, failure := range failures {
		responses = append(responses, newFailureResponse(failure))
	}

	apiJSON(rw, http.StatusOK, listFailuresResponse{Failures: responses, Total: total})
}
