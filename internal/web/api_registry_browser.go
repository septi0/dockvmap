package web

import (
	"net/http"
)

func (w *Web) apiListBrowsableRegistries(rw http.ResponseWriter, r *http.Request) {
	ids, err := w.registryBrowser.Browsable(r.Context())

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusOK, map[string][]int64{"registryIds": ids})
}

func (w *Web) apiListRegistryRepositories(rw http.ResponseWriter, r *http.Request) {
	registryID, err := parseRegistryID(r.PathValue("id"))

	if err != nil {
		apiError(rw, http.StatusBadRequest, "registry id is required")
		return
	}

	result, err := w.registryBrowser.Repositories(r.Context(), registryID)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	if result == nil {
		apiError(rw, http.StatusNotFound, "registry not found")
		return
	}

	apiJSON(rw, http.StatusOK, newRegistryRepositoriesResponse(*result))
}
