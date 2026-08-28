package web

import (
	"net/http"
	"time"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiTagRefreshStatus(rw http.ResponseWriter, r *http.Request) {
	interval, err := time.ParseDuration(w.cfg.TagsCheckInterval)
	enabled := err == nil && interval > 0

	var lastRun, nextDue *time.Time

	if enabled {
		at, ok, err := w.workerSchedule.LastRun(r.Context(), service.WorkerJobTagRefresh)

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "failed to read tag refresh status")

			return
		}

		if ok {
			at = at.UTC()
			due := at.Add(interval)
			lastRun = &at
			nextDue = &due
		}
	}

	apiJSON(rw, http.StatusOK, newTagRefreshStatusResponse(enabled, w.cfg.TagsCheckInterval, lastRun, nextDue))
}
