package web

import (
	"errors"
	"net/http"

	"github.com/septi0/dockvmap/internal/service"
)

func apiServiceError(rw http.ResponseWriter, err error) {
	var groupTagErr *service.GroupTagUnavailableError

	switch {
	case errors.As(err, &groupTagErr),
		errors.Is(err, service.ErrImageGroupBusy),
		errors.Is(err, service.ErrImageInAnotherGroup):
		apiError(rw, http.StatusConflict, err.Error())

	case errors.Is(err, service.ErrInvalidImage),
		errors.Is(err, service.ErrInvalidImageGroup),
		errors.Is(err, service.ErrInvalidRegistry),
		errors.Is(err, service.ErrInvalidUser),
		errors.Is(err, service.ErrInvalidProxyToken),
		errors.Is(err, service.ErrInvalidHost):
		apiError(rw, http.StatusBadRequest, err.Error())

	case errors.Is(err, service.ErrRegistryManagedByConfig):
		apiError(rw, http.StatusConflict, err.Error())

	case errors.Is(err, service.ErrHostAlreadyExists):
		apiError(rw, http.StatusConflict, "a host with this name already exists")

	case errors.Is(err, service.ErrHostAddressInUse):
		apiError(rw, http.StatusConflict, "a host with this address already exists")

	case errors.Is(err, service.ErrHostNotFound):
		apiError(rw, http.StatusNotFound, "host not found")

	case errors.Is(err, service.ErrUpstreamNotFound), errors.Is(err, service.ErrUpstreamUnauthorized):
		apiError(rw, http.StatusNotFound, "repository does not exist or may require authentication")

	case errors.Is(err, service.ErrUpstreamUnavailable):
		apiError(rw, http.StatusBadGateway, "upstream registry check failed")

	default:
		apiError(rw, http.StatusInternalServerError, "internal server error")
	}
}
