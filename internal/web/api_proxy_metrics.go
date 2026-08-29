package web

import (
	"net/http"
	"time"
)

func (w *Web) apiProxyMetrics(rw http.ResponseWriter, r *http.Request) {
	summary, err := w.proxyMetricsHistory.Summary(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "failed to load proxy metrics")

		return
	}

	response := proxyMetricsResponse{
		GeneratedAt: time.Now().UTC(),
		Windows:     summary,
	}

	if w.cacheUsage != nil {
		used, max, err := w.cacheUsage.Usage(r.Context())

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "failed to load cache usage")

			return
		}

		response.Cache = &proxyCacheUsageResponse{UsedBytes: used, MaxBytes: max}
	}

	apiJSON(rw, http.StatusOK, response)
}
