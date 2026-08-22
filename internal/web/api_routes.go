package web

import (
	"encoding/json"
	"net/http"
)

func (w *Web) registerAPIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/proxy/pull-info", apiMethod(http.MethodGet, w.apiProxyPullInfo))

	mux.HandleFunc("/version", apiMethod(http.MethodGet, w.apiVersion))

	mux.HandleFunc("/smtp-status", apiMethod(http.MethodGet, w.apiSMTPStatus))

	mux.HandleFunc("/proxy-auth-status", apiMethod(http.MethodGet, w.apiProxyAuthStatus))

	mux.HandleFunc("/proxy-metrics", apiMethod(http.MethodGet, w.apiProxyMetrics))

	mux.HandleFunc("/recent-failures", apiMethod(http.MethodGet, w.apiRecentFailures))

	mux.HandleFunc("/events", apiMethod(http.MethodGet, w.apiListEvents))

	mux.HandleFunc("/audit-logs", apiMethod(http.MethodGet, w.apiListAuditLogs))

	mux.HandleFunc("/setup", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.apiSetupStatus(rw, r)

		case http.MethodPost:
			w.apiBootstrapUser(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/login", apiMethod(http.MethodPost, w.apiLogin))

	mux.HandleFunc("/logout", apiMethod(http.MethodPost, w.apiLogout))

	mux.HandleFunc("/me", apiMethod(http.MethodGet, w.apiCurrentUser))

	mux.HandleFunc("/users/password", apiMethod(http.MethodPut, w.apiUpdateUserPassword))

	mux.HandleFunc("/users/email", apiMethod(http.MethodPut, w.apiUpdateUserEmail))

	mux.HandleFunc("/users/preferences", apiMethod(http.MethodPut, w.apiUpdateUserPreferences))

	mux.HandleFunc("/sessions", apiMethod(http.MethodGet, w.apiListSessions))

	mux.HandleFunc("/sessions/{id}", apiMethod(http.MethodDelete, w.apiInvalidateSession))

	mux.HandleFunc("/registries", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.apiListRegistries(rw, r)

		case http.MethodPost:
			w.apiCreateRegistry(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/registries/{id}", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.apiUpdateRegistry(rw, r)

		case http.MethodDelete:
			w.apiDeleteRegistry(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/proxy-tokens", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.apiListProxyTokens(rw, r)

		case http.MethodPost:
			w.apiCreateProxyToken(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/proxy-tokens/{id}", apiMethod(http.MethodDelete, w.apiDeleteProxyToken))

	mux.HandleFunc("/images", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.apiListImages(rw, r)

		case http.MethodPost:
			w.apiCreateImage(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/images/inspect", apiMethod(http.MethodPost, w.apiInspectRepository))

	mux.HandleFunc("/images/{id}", func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.apiGetImage(rw, r)

		case http.MethodDelete:
			w.apiDeleteImage(rw, r)

		default:
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/images/{id}/tags", apiMethod(http.MethodGet, w.apiGetImageTags))

	mux.HandleFunc("/images/{id}/refresh-tags", apiMethod(http.MethodPost, w.apiRefreshImageTags))

	mux.HandleFunc("/images/{id}/mark-seen", apiMethod(http.MethodPost, w.apiMarkImageTagsAsSeen))

	mux.HandleFunc("/images/{id}/tag", apiMethod(http.MethodPut, w.apiUpdateImageTag))

	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		apiError(rw, http.StatusNotFound, "API endpoint not found")
	})
}

func apiMethod(method string, handler http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			apiError(rw, http.StatusMethodNotAllowed, "method not allowed")

			return
		}

		handler(rw, r)
	}
}

func apiJSON(rw http.ResponseWriter, status int, value any) {
	rw.Header().Set("Content-Type", "application/json")

	rw.WriteHeader(status)

	_ = json.NewEncoder(rw).Encode(value)
}

func apiError(rw http.ResponseWriter, status int, message string) {
	apiJSON(rw, status, map[string]string{"error": message})
}

func decodeJSON[T any](rw http.ResponseWriter, r *http.Request) (T, bool) {
	var value T

	decoder := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&value); err != nil {
		apiError(rw, http.StatusBadRequest, "invalid JSON")

		return value, false
	}

	return value, true
}
