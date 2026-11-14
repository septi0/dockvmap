package replication

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/ocicopy"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

const userAgentPrefix = "dockvmap/"

type primaryService interface {
	Authenticate(ctx context.Context, secret string) (*model.ReplicationToken, error)
	State(ctx context.Context, req service.StateRequest) (*replicationapi.StateResponse, *replicationapi.Conflict, error)
	Changes(ctx context.Context, repository string, after int64) (*replicationapi.ChangesResponse, error)
	RecordPulls(ctx context.Context, report replicationapi.PullReport) error
	SeeFollower(token *model.ReplicationToken, address, version, status string, applied int64)
}

type handler struct {
	service  primaryService
	registry http.Handler
	allowed  ipmatch.Set
}

// NewHandler serves the replication API; registry is nil when the primary has no internal registry, and an empty allowed set lets every address in
func NewHandler(svc primaryService, registry *ocicopy.Registry, allowed ipmatch.Set) http.Handler {
	h := &handler{service: svc, allowed: allowed}

	if registry != nil {
		h.registry = newRegistryReader(registry)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+replicationapi.StatePath, h.authenticated(h.state))
	mux.HandleFunc("GET "+replicationapi.ChangesPath, h.authenticated(h.changes))
	mux.HandleFunc("POST "+replicationapi.PullsPath, h.authenticated(h.pulls))
	mux.HandleFunc(replicationapi.RegistryPrefix+"/", h.authenticated(h.registryRead))

	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, replicationapi.Error{Error: message})
}

type tokenKey struct{}

// authenticated checks the address before the token, so refused addresses cost no lookup and no log line
func (h *handler) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if address, ok := h.refusedAddress(r); ok {
			writeError(w, http.StatusForbidden, "replication isn't allowed from "+address+"; add it to replication.allowed_ips on the primary")
			return
		}

		secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")

		if !ok {
			writeError(w, http.StatusUnauthorized, "a replication token is required")
			return
		}

		token, err := h.service.Authenticate(r.Context(), secret)

		if err != nil {
			slog.Error("checking a replication token failed", "error", err)
			writeError(w, http.StatusInternalServerError, "checking the replication token failed")
			return
		}

		if token == nil {
			slog.Warn("refused a replication request with an unknown token", "remote", r.RemoteAddr)
			writeError(w, http.StatusUnauthorized, "this replication token isn't valid on this primary; create a new one on its System > Replication page")
			return
		}

		next(w, r.WithContext(context.WithValue(r.Context(), tokenKey{}, token)))
	}
}

func (h *handler) refusedAddress(r *http.Request) (string, bool) {
	if h.allowed.Empty() {
		return "", false
	}

	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)

	return addrPort.Addr().Unmap().String(), err != nil || !h.allowed.Contains(addrPort.Addr())
}

func (h *handler) state(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	req := service.StateRequest{
		Tag:     query.Get(replicationapi.ParamTag),
		Cluster: query.Get(replicationapi.ParamCluster),
		Full:    query.Get(replicationapi.ParamFull) == "1",
	}

	if raw := query.Get(replicationapi.ParamVersion); raw != "" {
		version, err := strconv.ParseInt(raw, 10, 64)

		if err != nil {
			writeError(w, http.StatusBadRequest, "version must be a number")
			return
		}

		req.Version, req.HasVersion = version, true
	}

	address, _, _ := net.SplitHostPort(r.RemoteAddr)
	token := r.Context().Value(tokenKey{}).(*model.ReplicationToken)
	h.service.SeeFollower(token, address, strings.TrimPrefix(r.UserAgent(), userAgentPrefix), r.Header.Get(replicationapi.FollowerStatusHeader), req.Version)

	response, conflict, err := h.service.State(r.Context(), req)

	switch {
	case err != nil:
		slog.Error("answering a follower failed", "error", err)
		writeError(w, http.StatusInternalServerError, "reading the replication state failed; check the primary's log")
	case conflict != nil:
		writeJSON(w, http.StatusConflict, conflict)
	case response == nil:
		w.WriteHeader(http.StatusNotModified)
	default:
		writeJSON(w, http.StatusOK, response)
	}
}

func (h *handler) changes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	after, err := strconv.ParseInt(query.Get(replicationapi.ParamAfter), 10, 64)
	repository := query.Get(replicationapi.ParamRepository)

	if err != nil || repository == "" {
		writeError(w, http.StatusBadRequest, "repository and a numeric after are required")
		return
	}

	response, err := h.service.Changes(r.Context(), repository, after)

	if err != nil {
		slog.Error("reading registry changes for a follower failed", "repository", repository, "error", err)
		writeError(w, http.StatusInternalServerError, "reading the changes failed; check the primary's log")
		return
	}

	writeJSON(w, http.StatusOK, response)
}

const maxPullReportBytes = 8 << 20

func (h *handler) pulls(w http.ResponseWriter, r *http.Request) {
	var report replicationapi.PullReport

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPullReportBytes)).Decode(&report); err != nil {
		writeError(w, http.StatusBadRequest, "the pull report isn't valid JSON")
		return
	}

	if err := h.service.RecordPulls(r.Context(), report); err != nil {
		slog.Error("recording a follower's pulls failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recording the pulls failed; check the primary's log")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) registryRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "the replication API only reads the registry")
		return
	}

	if h.registry == nil {
		writeError(w, http.StatusNotFound, "this primary has no internal registry")
		return
	}

	if !strings.HasPrefix(strings.TrimPrefix(r.URL.Path, replicationapi.RegistryPrefix), "/v2/") {
		writeError(w, http.StatusNotFound, "not a registry path")
		return
	}

	h.registry.ServeHTTP(w, r)
}

func newRegistryReader(registry *ocicopy.Registry) http.Handler {
	target, _ := url.Parse(registry.BaseURL)

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, replicationapi.RegistryPrefix)
			pr.Out.URL.RawPath = ""
			pr.Out.Header.Del("Authorization")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() != nil {
				w.WriteHeader(http.StatusBadGateway)
				return
			}

			slog.Warn("reading the internal registry for a follower failed", "registry", target.Host, "error", err)
			writeError(w, http.StatusBadGateway, "the primary's internal registry didn't answer")
		},
		ErrorLog:  slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug),
		Transport: registry.Client.Transport,
	}
}
