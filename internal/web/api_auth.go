package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiLogin(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[loginRequest](rw, r)
	if !ok {
		return
	}

	token, expiresAt, err := w.sessions.Login(r.Context(), request.Username, request.Password)

	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			apiError(rw, http.StatusUnauthorized, "invalid username or password")
			return
		}

		if errors.Is(err, service.ErrLoginRateLimited) {
			rw.Header().Set("Retry-After", strconv.Itoa(int(w.loginRateLimitWindow.Seconds())))
			apiError(rw, http.StatusTooManyRequests, "too many failed login attempts, try again later")
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	setSessionCookie(rw, token, expiresAt, w.cfg.SecureCookies)

	apiJSON(rw, http.StatusOK, map[string]string{"status": "logged in"})
}

func (w *Web) apiLogout(rw http.ResponseWriter, r *http.Request) {
	var token string

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		token = cookie.Value
	}

	if err := w.sessions.Logout(r.Context(), token); err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	clearSessionCookie(rw, w.cfg.SecureCookies)

	apiJSON(rw, http.StatusOK, map[string]string{"status": "logged out"})
}

func (w *Web) apiCurrentUser(rw http.ResponseWriter, r *http.Request) {
	user, err := w.users.GetProfile(r.Context())

	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			apiError(rw, http.StatusUnauthorized, "authentication required")
			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusOK, newCurrentUserResponse(*user))
}
