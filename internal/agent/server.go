package agent

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

var errPairedElsewhere = errors.New("paired with another DockVMap")

func (a *Agent) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(agentapi.AdoptPath, a.handleAdopt)
	mux.HandleFunc(agentapi.SnapshotPath, a.handleSnapshot)

	return a.authorize(mux)
}

func (a *Agent) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := callerFingerprint(r)

		if caller == "" {
			writeError(w, http.StatusUnauthorized, "a client certificate is required")
			return
		}

		paired := a.pairedWith()

		switch {
		case paired != "" && caller != paired:
			slog.Warn("refused a request from a DockVMap this agent is not paired with", "remote", r.RemoteAddr, "fingerprint", caller)
			writeError(w, http.StatusForbidden, "this agent is paired with another DockVMap")

		case paired == "" && r.URL.Path != agentapi.AdoptPath:
			writeError(w, http.StatusConflict, "this agent is waiting to be adopted")

		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (a *Agent) handleAdopt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch err := a.pair(callerFingerprint(r), r.RemoteAddr); {
	case errors.Is(err, errPairedElsewhere):
		writeError(w, http.StatusForbidden, "this agent is paired with another DockVMap")

	case err != nil:
		slog.Error("saving the pairing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "the agent could not save the pairing; check its data volume")

	default:
		writeJSON(w, http.StatusOK, agentapi.Adoption{ProtocolVersion: agentapi.ProtocolVersion, AgentVersion: a.version})
	}
}

func (a *Agent) pair(caller, remote string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.paired.DockVMap == caller {
		return nil
	}

	if a.paired.DockVMap != "" {
		return errPairedElsewhere
	}

	paired := pairing{DockVMap: caller, PairedAt: time.Now().UTC()}

	if err := savePairing(a.cfg.DataPath, paired); err != nil {
		return err
	}

	a.paired = paired
	slog.Info("paired with DockVMap", "dockvmap", caller, "remote", remote)

	return nil
}

func (a *Agent) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	proxyHost := r.Header.Get(agentapi.ProxyHostHeader)

	if len(proxyHost) > agentapi.MaxTextLength {
		writeError(w, http.StatusBadRequest, "proxy host is too long")
		return
	}

	report := a.snapshot(r.Context(), proxyHost)
	etag := `"` + report.Fingerprint + `"`

	w.Header().Set("ETag", etag)

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func callerFingerprint(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}

	return agentapi.Fingerprint(r.TLS.PeerCertificates[0])
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("writing a response failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, agentapi.Error{Error: message})
}
