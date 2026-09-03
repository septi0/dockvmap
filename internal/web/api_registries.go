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

	updated, err := w.registries.UpdateByID(r.Context(), registryEntry)

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

	info, err := w.registries.GetByID(r.Context(), registryID)

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

	deleted, err := w.registries.DeleteByID(r.Context(), registryID)

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

type testRegistryRequest struct {
	Registry   string                `json:"registry"`
	Username   string                `json:"username"`
	Credential string                `json:"credential"`
	Options    model.RegistryOptions `json:"options"`
}

type testExistingRegistryRequest struct {
	Registry   string                `json:"registry"`
	Username   *string               `json:"username"`
	Credential *string               `json:"credential"`
	Options    model.RegistryOptions `json:"options"`
}

type registryTestResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (w *Web) apiTestRegistry(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[testRegistryRequest](rw, r)
	if !ok {
		return
	}

	err := w.registries.TestConnection(
		r.Context(),
		strings.TrimSpace(request.Registry),
		strings.TrimSpace(request.Username),
		request.Credential,
		request.Options,
	)

	writeRegistryTestResult(rw, err)
}

func (w *Web) apiTestExistingRegistry(rw http.ResponseWriter, r *http.Request) {
	registryID, err := parseRegistryID(r.PathValue("id"))
	if err != nil {
		apiError(rw, http.StatusBadRequest, "registry id is required")
		return
	}

	request, ok := decodeJSON[testExistingRegistryRequest](rw, r)
	if !ok {
		return
	}

	err = w.registries.TestExistingConnection(r.Context(), registryID, service.RegistryConnTest{
		Host:       strings.TrimSpace(request.Registry),
		Username:   request.Username,
		Credential: request.Credential,
		Options:    request.Options,
	})

	writeRegistryTestResult(rw, err)
}

func writeRegistryTestResult(rw http.ResponseWriter, err error) {
	switch {
	case err == nil:
		apiJSON(rw, http.StatusOK, registryTestResponse{OK: true})

	case errors.Is(err, service.ErrInvalidRegistry):
		apiError(rw, http.StatusBadRequest, err.Error())

	case errors.Is(err, service.ErrRegistryNotFound):
		apiError(rw, http.StatusNotFound, "registry not found")

	default:
		apiJSON(rw, http.StatusOK, registryTestResponse{OK: false, Error: err.Error()})
	}
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
