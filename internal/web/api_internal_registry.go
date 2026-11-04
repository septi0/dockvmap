package web

import (
	"log/slog"
	"net/http"
)

func (w *Web) apiListInternalRepositories(rw http.ResponseWriter, r *http.Request) {
	if w.internalRegistry.Address() == "" {
		apiError(rw, http.StatusNotFound, "the internal registry is not enabled")
		return
	}

	repositories, err := w.internalRegistry.Repositories(r.Context())

	if err != nil {
		slog.Warn("listing the internal registry's repositories failed", "error", err)
		apiError(rw, http.StatusBadGateway, err.Error())
		return
	}

	apiJSON(rw, http.StatusOK, repositories)
}
