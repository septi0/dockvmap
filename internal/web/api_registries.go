package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

const errCredentialEncryptionNotConfiguredMsg = "registry credential encryption is not configured on the server - set credential_encryption_key in config.yaml"

func (w *Web) apiListRegistries(rw http.ResponseWriter, r *http.Request) {
	registries, err := w.registries.List(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]registryResponse, 0, len(registries))

	for _, registry := range registries {
		responses = append(responses, newRegistryResponse(registry))
	}

	apiJSON(rw, http.StatusOK, responses)
}

func (w *Web) apiCreateRegistry(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[createRegistryRequest](rw, r)
	if !ok {
		return
	}

	registryEntry := model.Registry{
		Registry:   strings.TrimSpace(request.Registry),
		Username:   strings.TrimSpace(request.Username),
		Credential: request.Credential,
		Options:    request.Options,
	}

	registryID, err := w.registries.Create(r.Context(), registryEntry)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRegistry):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrRegistryAlreadyExists):
			apiError(rw, http.StatusConflict, "registry already exists")

		case errors.Is(err, service.ErrCredentialEncryptionNotConfigured):
			apiError(rw, http.StatusInternalServerError, errCredentialEncryptionNotConfiguredMsg)

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusCreated, registryResponse{
		ID:                       registryID,
		Registry:                 registryEntry.Registry,
		Username:                 registryEntry.Username,
		AuthenticationConfigured: registryEntry.Username != "",
		Options:                  registryEntry.Options,
	})
}

func (w *Web) apiUpdateRegistry(rw http.ResponseWriter, r *http.Request) {
	registryID, err := parseRegistryID(r.PathValue("id"))
	if err != nil {
		apiError(rw, http.StatusBadRequest, "registry id is required")
		return
	}

	request, ok := decodeJSON[updateRegistryRequest](rw, r)
	if !ok {
		return
	}

	registryEntry := model.RegistryUpdate{
		ID:         registryID,
		Registry:   strings.TrimSpace(request.Registry),
		Username:   request.Username,
		Credential: request.Credential,
		Options:    request.Options,
	}

	updated, err := w.registries.Update(r.Context(), registryEntry)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidRegistry):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrRegistryAlreadyExists):
			apiError(rw, http.StatusConflict, "registry already exists")

		case errors.Is(err, service.ErrCredentialEncryptionNotConfigured):
			apiError(rw, http.StatusInternalServerError, errCredentialEncryptionNotConfiguredMsg)

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	if !updated {
		apiError(rw, http.StatusNotFound, "registry not found")
		return
	}

	info, err := w.registries.Get(r.Context(), registryID)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	if info == nil {
		apiError(rw, http.StatusNotFound, "registry not found")
		return
	}

	apiJSON(rw, http.StatusOK, newRegistryResponse(*info))
}

func (w *Web) apiDeleteRegistry(rw http.ResponseWriter, r *http.Request) {
	registryID, err := parseRegistryID(r.PathValue("id"))
	if err != nil {
		apiError(rw, http.StatusBadRequest, "registry id is required")
		return
	}

	deleted, err := w.registries.Delete(r.Context(), registryID)

	if err != nil {
		if errors.Is(err, service.ErrInvalidRegistry) {
			apiError(rw, http.StatusBadRequest, err.Error())
			return
		}

		if errors.Is(err, service.ErrRegistryInUse) {
			apiError(rw, http.StatusConflict, "registry is used by one or more images")
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	if !deleted {
		apiError(rw, http.StatusNotFound, "registry not found")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func parseRegistryID(value string) (int64, error) {
	if value == "" {
		return 0, errors.New("registry id is required")
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("registry id must be a positive integer")
	}

	return id, nil
}
