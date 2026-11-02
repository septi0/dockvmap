package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/service"
)

const maxAgentReportBytes = 4 << 20

type agentHostKey struct{}

func (w *Web) registerAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/config", apiMethod(http.MethodGet, w.apiAgentConfig))
	mux.HandleFunc("/v1/report", apiMethod(http.MethodPost, w.apiAgentReport))
	mux.HandleFunc("/v1/heartbeat", apiMethod(http.MethodPost, w.apiAgentHeartbeat))
}

func (w *Web) requireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

		if !ok {
			apiError(rw, http.StatusUnauthorized, "agent token required")
			return
		}

		hostID, found, err := w.hosts.Authenticate(r.Context(), strings.TrimSpace(token))

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "internal server error")
			return
		}

		if !found {
			slog.Debug("agent request with an unknown token", "ip", resolveClientIP(r, w.trustedProxies))
			apiError(rw, http.StatusUnauthorized, "invalid agent token")
			return
		}

		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), agentHostKey{}, hostID)))
	})
}

func (w *Web) apiAgentConfig(rw http.ResponseWriter, r *http.Request) {
	apiJSON(rw, http.StatusOK, agentapi.Config{ProxyHost: w.proxyTarget(r).Host})
}

func (w *Web) apiAgentReport(rw http.ResponseWriter, r *http.Request) {
	var report agentapi.Report

	if !decodeAgentBody(rw, r, &report) {
		return
	}

	hostID, _ := r.Context().Value(agentHostKey{}).(int64)

	agentResult(rw, hostID, w.hosts.Report(r.Context(), hostID, report))
}

func (w *Web) apiAgentHeartbeat(rw http.ResponseWriter, r *http.Request) {
	var heartbeat agentapi.Heartbeat

	if !decodeAgentBody(rw, r, &heartbeat) {
		return
	}

	hostID, _ := r.Context().Value(agentHostKey{}).(int64)

	agentResult(rw, hostID, w.hosts.Heartbeat(r.Context(), hostID, heartbeat))
}

func decodeAgentBody(rw http.ResponseWriter, r *http.Request, out any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxAgentReportBytes)).Decode(out); err != nil {
		var tooLarge *http.MaxBytesError

		if errors.As(err, &tooLarge) {
			apiError(rw, http.StatusRequestEntityTooLarge, "report is too large")
			return false
		}

		apiError(rw, http.StatusBadRequest, "invalid JSON")
		return false
	}

	return true
}

func agentResult(rw http.ResponseWriter, hostID int64, err error) {
	switch {
	case err == nil:
		apiJSON(rw, http.StatusOK, map[string]string{"status": "ok"})

	case errors.Is(err, service.ErrFullReportNeeded):
		apiError(rw, agentapi.FullReportNeededStatus, err.Error())

	case errors.Is(err, service.ErrHostNotFound):
		apiError(rw, http.StatusUnauthorized, "invalid agent token")

	case errors.Is(err, service.ErrInvalidAgentReport), errors.Is(err, service.ErrUnsupportedAgentProtocol):
		slog.Warn("agent report rejected", "host", hostID, "error", err)
		apiError(rw, http.StatusBadRequest, err.Error())

	default:
		slog.Error("storing agent report failed", "host", hostID, "error", err)
		apiError(rw, http.StatusInternalServerError, "internal server error")
	}
}
