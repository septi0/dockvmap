package web

import (
	"net/http"

	"github.com/septi0/dockvmap/internal/model"
)

type dashboardSummaryResponse struct {
	Images struct {
		Total           int64 `json:"total"`
		UpdateAvailable int64 `json:"updateAvailable"`
		FailedCheck     int64 `json:"failedCheck"`
	} `json:"images"`
}

func (w *Web) apiDashboardSummary(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp dashboardSummaryResponse

	counts := []struct {
		status model.ImageStatusFilter
		target *int64
	}{
		{"", &resp.Images.Total},
		{model.ImageStatusUpdateAvailable, &resp.Images.UpdateAvailable},
		{model.ImageStatusFailedCheck, &resp.Images.FailedCheck},
	}

	for _, c := range counts {
		count, err := w.images.Count(ctx, model.ImageListFilters{Status: c.status})

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "failed to load dashboard summary")

			return
		}

		*c.target = count
	}

	apiJSON(rw, http.StatusOK, resp)
}
