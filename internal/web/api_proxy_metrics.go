package web

import "net/http"

func (w *Web) apiProxyMetrics(rw http.ResponseWriter, r *http.Request) {
	snapshot := w.proxyMetrics.Snapshot()

	apiJSON(rw, http.StatusOK, newProxyMetricsResponse(snapshot, w.cfg.BlobCache.Enabled))
}
