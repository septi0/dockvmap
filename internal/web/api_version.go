package web

import "net/http"

func (w *Web) apiVersion(rw http.ResponseWriter, r *http.Request) {
	apiJSON(rw, http.StatusOK, map[string]string{"version": w.version})
}
