package web

import (
	"errors"
	"net/http"

	"github.com/septi0/dockvmap/internal/service"
)

func apiServiceError(rw http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidImage),
		errors.Is(err, service.ErrInvalidRegistry),
		errors.Is(err, service.ErrInvalidUser),
		errors.Is(err, service.ErrInvalidProxyToken):
		apiError(rw, http.StatusBadRequest, err.Error())

	case errors.Is(err, service.ErrUpstreamNotFound), errors.Is(err, service.ErrUpstreamUnauthorized):
		apiError(rw, http.StatusNotFound, "repository does not exist or may require authentication")

	case errors.Is(err, service.ErrUpstreamUnavailable):
		apiError(rw, http.StatusBadGateway, "upstream registry check failed")

	default:
		apiError(rw, http.StatusInternalServerError, "internal server error")
	}
}
