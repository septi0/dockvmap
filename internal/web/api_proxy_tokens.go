package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiListProxyTokens(rw http.ResponseWriter, r *http.Request) {
	tokens, err := w.proxyTokens.List(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]proxyTokenResponse, 0, len(tokens))

	for _, token := range tokens {
		responses = append(responses, newProxyTokenResponse(token))
	}

	apiJSON(rw, http.StatusOK, responses)
}

func (w *Web) apiCreateProxyToken(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[createProxyTokenRequest](rw, r)
	if !ok {
		return
	}

	id, token, err := w.proxyTokens.Create(r.Context(), request.Label)

	if err != nil {
		if errors.Is(err, service.ErrInvalidProxyToken) {
			apiError(rw, http.StatusBadRequest, err.Error())
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusCreated, createProxyTokenResponse{
		ID:    id,
		Label: request.Label,
		Token: token,
	})
}

func (w *Web) apiDeleteProxyToken(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "proxy token id must be a valid integer")
		return
	}

	deleted, err := w.proxyTokens.Delete(r.Context(), id)

	if err != nil {
		if errors.Is(err, service.ErrInvalidProxyToken) {
			apiError(rw, http.StatusBadRequest, err.Error())
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	if !deleted {
		apiError(rw, http.StatusNotFound, "proxy token not found")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}
