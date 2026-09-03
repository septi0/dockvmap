package web

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

type systemDatabaseStatus struct {
	Reachable     bool   `json:"reachable"`
	SchemaVersion int    `json:"schemaVersion"`
	SizeBytes     int64  `json:"sizeBytes"`
	Path          string `json:"path"`
}

type systemStatusResponse struct {
	Version          string               `json:"version"`
	StartedAt        time.Time            `json:"startedAt"`
	DataPath         string               `json:"dataPath"`
	Database         systemDatabaseStatus `json:"database"`
	ConfigWarnings   []string             `json:"configWarnings"`
	VirtualTag       string               `json:"virtualTag"`
	TrustedProxies   []string             `json:"trustedProxies"`
	ProxyAuthEnabled bool                 `json:"proxyAuthEnabled"`
}

func (w *Web) apiSystemStatus(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	dbPath := filepath.Join(w.dataPath, "dockvmap.db")

	status := systemStatusResponse{
		Version:          w.version,
		StartedAt:        w.startedAt,
		DataPath:         w.dataPath,
		ConfigWarnings:   append([]string{}, w.cfg.DerivedWarnings...),
		VirtualTag:       w.cfg.VirtualTag,
		TrustedProxies:   append([]string{}, w.resolvedProxies...),
		ProxyAuthEnabled: w.cfg.ProxyAuth.Enabled,
	}

	status.Database.Path = dbPath
	status.Database.Reachable = w.health.Ping(ctx) == nil

	if version, err := w.health.SchemaVersion(ctx); err == nil {
		status.Database.SchemaVersion = version
	}

	if info, err := os.Stat(dbPath); err == nil {
		status.Database.SizeBytes = info.Size()
	}

	apiJSON(rw, http.StatusOK, status)
}

type systemTaskResponse struct {
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	IntervalSeconds float64    `json:"intervalSeconds"`
	Enabled         bool       `json:"enabled"`
	DisabledReason  string     `json:"disabledReason,omitempty"`
	Triggerable     bool       `json:"triggerable"`
	Running         bool       `json:"running"`
	LastRun         *time.Time `json:"lastRun,omitempty"`
	NextDue         *time.Time `json:"nextDue,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	LastCount       *int64     `json:"lastCount,omitempty"`
}

type systemTasksResponse struct {
	Tasks []systemTaskResponse `json:"tasks"`
}

func (w *Web) apiSystemTasks(rw http.ResponseWriter, r *http.Request) {
	ticks, err := w.worker.Ticks(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	byJob := make(map[string]model.WorkerTick, len(ticks))

	for _, tick := range ticks {
		byJob[tick.Job] = tick
	}

	jobs := w.worker.Catalog()
	tasks := make([]systemTaskResponse, 0, len(jobs))

	for _, job := range jobs {
		task := systemTaskResponse{
			Name:            job.Name,
			Description:     job.Description,
			IntervalSeconds: job.Interval.Seconds(),
			Enabled:         job.Enabled,
			DisabledReason:  job.DisabledReason,
			Triggerable:     job.Triggerable,
			Running:         w.worker.Running(job.Name),
		}

		if tick, ok := byJob[job.Name]; ok {
			lastRun := tick.LastRunAt
			task.LastRun = &lastRun
			task.LastError = tick.LastError
			task.LastCount = tick.LastCount

			if job.Enabled && job.Interval > 0 {
				nextDue := lastRun.Add(job.Interval)
				if now := time.Now(); nextDue.Before(now) {
					nextDue = now
				}
				task.NextDue = &nextDue
			}
		}

		tasks = append(tasks, task)
	}

	apiJSON(rw, http.StatusOK, systemTasksResponse{Tasks: tasks})
}

func (w *Web) apiRunSystemTask(rw http.ResponseWriter, r *http.Request) {
	if !w.worker.Trigger(r.PathValue("name")) {
		apiError(rw, http.StatusNotFound, "unknown or non-triggerable task")
		return
	}

	apiJSON(rw, http.StatusAccepted, map[string]string{"status": "triggered"})
}
