package web

import (
	"errors"
	"net/http"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiSetupStatus(rw http.ResponseWriter, r *http.Request) {
	required, err := w.users.SetupRequired(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]bool{"required": required})
}

func (w *Web) apiBootstrapUser(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[bootstrapUserRequest](rw, r)
	if !ok {
		return
	}

	userID, err := w.users.Bootstrap(r.Context(), request.Username, request.Email, request.Password)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUser):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrUsernameConflict):
			apiError(rw, http.StatusConflict, "username already exists")

		case errors.Is(err, service.ErrSetupComplete):
			apiError(rw, http.StatusForbidden, "initial setup has already been completed")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusCreated, map[string]any{"id": userID, "status": "created"})
}

func (w *Web) apiUpdateUserPassword(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[updateUserPasswordRequest](rw, r)
	if !ok {
		return
	}

	err := w.users.UpdatePassword(r.Context(), request.CurrentPassword, request.NewPassword)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUser):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrIncorrectCurrentPassword):
			apiError(rw, http.StatusForbidden, "current password is incorrect")

		case errors.Is(err, service.ErrUserNotFound):
			apiError(rw, http.StatusUnauthorized, "authentication required")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) apiUpdateUserEmail(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[updateUserEmailRequest](rw, r)
	if !ok {
		return
	}

	err := w.users.UpdateEmail(r.Context(), request.Email)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidUser):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrUserNotFound):
			apiError(rw, http.StatusUnauthorized, "authentication required")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) apiUpdateUserPreferences(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[updateUserPreferencesRequest](rw, r)
	if !ok {
		return
	}

	var notifyLevel *model.NotifyLevel
	if request.NotifyLevel != nil {
		level := model.NotifyLevel(*request.NotifyLevel)
		notifyLevel = &level
	}

	err := w.users.UpdatePreferences(r.Context(), model.UserPreferencesUpdate{
		NotifyLevel: notifyLevel,
	})

	if err != nil {
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			apiError(rw, http.StatusUnauthorized, "authentication required")
		case errors.Is(err, service.ErrInvalidUser):
			apiError(rw, http.StatusBadRequest, err.Error())
		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}
