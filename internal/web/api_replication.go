package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

const (
	roleStandalone = "standalone"
	rolePrimary    = "primary"
	roleFollower   = "follower"
)

type replicationStatusResponse struct {
	Role    string                      `json:"role"`
	Primary *primaryReplicationResponse `json:"primary,omitempty"`
}

type primaryReplicationResponse struct {
	Cluster      string                 `json:"cluster"`
	StateVersion int64                  `json:"stateVersion"`
	Fingerprint  string                 `json:"fingerprint"`
	Listen       string                 `json:"listen"`
	AllowedIPs   []string               `json:"allowedIps"`
	Followers    []seenFollowerResponse `json:"followers"`
}

type seenFollowerResponse struct {
	ID             int64                          `json:"id"`
	Name           string                         `json:"name"`
	Address        string                         `json:"address"`
	Token          string                         `json:"token"`
	Version        string                         `json:"version"`
	AppliedVersion int64                          `json:"appliedVersion"`
	Status         string                         `json:"status"`
	Connection     string                         `json:"connection"`
	FirstSeen      time.Time                      `json:"firstSeen"`
	LastSeen       time.Time                      `json:"lastSeen"`
	Health         *replicationapi.FollowerHealth `json:"health,omitempty"`
}

func newSeenFollowerResponses(followers []service.SeenFollower) []seenFollowerResponse {
	responses := make([]seenFollowerResponse, 0, len(followers))

	for _, f := range followers {
		responses = append(responses, seenFollowerResponse(f))
	}

	return responses
}

type replicationTokenResponse struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

func newPrimaryReplicationResponse(status *service.ReplicationStatus, allowedIPs []string) *primaryReplicationResponse {
	return &primaryReplicationResponse{
		Cluster:      status.ClusterID,
		StateVersion: status.Version,
		Fingerprint:  status.Fingerprint,
		Listen:       status.Listen,
		AllowedIPs:   append([]string{}, allowedIPs...),
		Followers:    newSeenFollowerResponses(status.Followers),
	}
}

func (w *Web) role() string {
	switch {
	case w.replication != nil:
		return rolePrimary
	case w.follower != nil:
		return roleFollower
	}

	return roleStandalone
}

type createReplicationTokenRequest struct {
	Label string `json:"label"`
}

type createReplicationTokenResponse struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Token string `json:"token"`
}

func (w *Web) apiReplicationStatus(rw http.ResponseWriter, r *http.Request) {
	response := replicationStatusResponse{Role: w.role()}

	if response.Role == rolePrimary {
		status, err := w.replication.Status(r.Context())

		if err != nil {
			apiError(rw, http.StatusInternalServerError, "internal server error")
			return
		}

		response.Primary = newPrimaryReplicationResponse(status, w.cfg.Replication.AllowedIPs)
	}

	apiJSON(rw, http.StatusOK, response)
}

func (w *Web) requirePrimary(rw http.ResponseWriter) bool {
	if w.replication == nil {
		apiError(rw, http.StatusNotFound, "replication is off on this DockVMap; turn it on with replication.enabled in config")
		return false
	}

	return true
}

func (w *Web) apiListReplicationTokens(rw http.ResponseWriter, r *http.Request) {
	if !w.requirePrimary(rw) {
		return
	}

	tokens, err := w.replication.ListTokens(r.Context())

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]replicationTokenResponse, 0, len(tokens))

	for _, token := range tokens {
		responses = append(responses, replicationTokenResponse(token))
	}

	apiJSON(rw, http.StatusOK, responses)
}

func (w *Web) apiCreateReplicationToken(rw http.ResponseWriter, r *http.Request) {
	if !w.requirePrimary(rw) {
		return
	}

	request, ok := decodeJSON[createReplicationTokenRequest](rw, r)

	if !ok {
		return
	}

	id, token, err := w.replication.CreateToken(r.Context(), request.Label)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusCreated, createReplicationTokenResponse{ID: id, Label: request.Label, Token: token})
}

func (w *Web) apiDeleteReplicationToken(rw http.ResponseWriter, r *http.Request) {
	if !w.requirePrimary(rw) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "replication token id must be a valid integer")
		return
	}

	deleted, err := w.replication.DeleteToken(r.Context(), id)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	if !deleted {
		apiError(rw, http.StatusNotFound, "replication token not found")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

type renameReplicationFollowerRequest struct {
	Name string `json:"name"`
}

func (w *Web) apiRenameReplicationFollower(rw http.ResponseWriter, r *http.Request) {
	if !w.requirePrimary(rw) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "follower id must be a valid integer")
		return
	}

	request, ok := decodeJSON[renameReplicationFollowerRequest](rw, r)

	if !ok {
		return
	}

	renamed, err := w.replication.RenameFollower(r.Context(), id, request.Name)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	if !renamed {
		apiError(rw, http.StatusNotFound, "follower not found")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "renamed"})
}

func (w *Web) apiRemoveReplicationFollower(rw http.ResponseWriter, r *http.Request) {
	if !w.requirePrimary(rw) {
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "follower id must be a valid integer")
		return
	}

	removed, err := w.replication.RemoveFollower(r.Context(), id)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	if !removed {
		apiError(rw, http.StatusNotFound, "follower not found")
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "removed"})
}
