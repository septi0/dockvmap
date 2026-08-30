package web

import (
	"log/slog"
	"net/http"

	"github.com/septi0/dockvmap/internal/service"
)

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("X-Frame-Options", "DENY")
		rw.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")

		next.ServeHTTP(rw, r)
	})
}

func (w *Web) withRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		slog.Info("clientip debug",
			"path", r.URL.Path,
			"remoteAddr", r.RemoteAddr,
			"xff", r.Header.Get("X-Forwarded-For"),
			"xRealIP", r.Header.Get("X-Real-IP"),
			"forwarded", r.Header.Get("Forwarded"),
			"resolved", resolveClientIP(r, w.trustedProxies))

		ctx := service.WithRequestInfo(r.Context(), service.RequestInfo{
			IP:        resolveClientIP(r, w.trustedProxies),
			UserAgent: r.UserAgent(),
		})

		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

var publicAPIPaths = map[string]bool{
	"/setup":  true,
	"/login":  true,
	"/logout": true,
}

func (w *Web) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if publicAPIPaths[r.URL.Path] {
			next.ServeHTTP(rw, r)
			return
		}

		cookie, err := r.Cookie(sessionCookieName)

		if err != nil {
			apiError(rw, http.StatusUnauthorized, "authentication required")
			return
		}

		currentUser, err := w.sessions.Validate(r.Context(), cookie.Value)

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "internal server error")
			return
		}

		if currentUser == nil {
			apiError(rw, http.StatusUnauthorized, "authentication required")
			return
		}

		ctx := service.WithCurrentUser(r.Context(), *currentUser)
		ctx = service.WithSessionToken(ctx, cookie.Value)

		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}
