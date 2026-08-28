package web

import "net/http"

func (w *Web) apiHealth(rw http.ResponseWriter, r *http.Request) {
	if err := w.health.Ping(r.Context()); err != nil {
		apiJSON(rw, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "version": w.version})
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "ok", "version": w.version})
}
