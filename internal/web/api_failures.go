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

	since, until, err := parseDateRange(r)

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

	failures, total, ok := listAndCount(rw, r.Context(), filters, w.failures.List, w.failures.Count)

	if !ok {
		return
	}

	responses := make([]failureResponse, 0, len(failures))

	for _, failure := range failures {
		responses = append(responses, newFailureResponse(failure))
	}

	apiJSON(rw, http.StatusOK, listFailuresResponse{Failures: responses, Total: total})
}
