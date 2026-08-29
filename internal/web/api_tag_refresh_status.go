package web

import (
	"net/http"
	"time"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiTriggerTagRefresh(rw http.ResponseWriter, r *http.Request) {
	if w.workerActivity.Running(service.WorkerJobTagRefresh) {
		apiError(rw, http.StatusConflict, "a tag check is already running")

		return
	}

	if !w.workerTrigger.Trigger(service.WorkerJobTagRefresh) {
		apiError(rw, http.StatusConflict, "automatic tag checks are disabled")

		return
	}

	apiJSON(rw, http.StatusAccepted, map[string]string{"status": "triggered"})
}

func (w *Web) apiTagRefreshStatus(rw http.ResponseWriter, r *http.Request) {
	interval := w.cfg.TagsCheckIntervalDuration()
	enabled := interval > 0

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

	running := w.workerActivity.Running(service.WorkerJobTagRefresh)

	apiJSON(rw, http.StatusOK, newTagRefreshStatusResponse(enabled, w.cfg.TagsCheckInterval, running, lastRun, nextDue))
}
