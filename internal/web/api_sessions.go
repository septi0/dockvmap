package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiListSessions(rw http.ResponseWriter, r *http.Request) {
	sessions, err := w.sessions.ListActive(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]sessionResponse, 0, len(sessions))

	for _, session := range sessions {
		responses = append(responses, newSessionResponse(session))
	}

	apiJSON(rw, http.StatusOK, map[string]any{"sessions": responses})
}

func (w *Web) apiInvalidateSession(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "session id must be a valid integer")
		return
	}

	err = w.sessions.InvalidateSession(r.Context(), id)

	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			apiError(rw, http.StatusNotFound, "session not found")
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "revoked"})
}
