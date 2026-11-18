package web

import (
	"context"
	"net/http"
	"time"

	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiHealth(rw http.ResponseWriter, r *http.Request) {
	if err := w.health.Ping(r.Context()); err != nil {
		apiJSON(rw, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "version": w.version})
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "ok", "version": w.version})
}

// below keepalived's usual script timeout, so a hung registry answers not ready instead of nothing
const readyCheckTimeout = 5 * time.Second

type readyResponse struct {
	Role   string            `json:"role"`
	Ready  bool              `json:"ready"`
	Checks map[string]string `json:"checks"`

	Cluster      string           `json:"cluster,omitempty"`
	StateVersion *int64           `json:"stateVersion,omitempty"`
	Followers    []followerHealth `json:"followers,omitempty"`

	*followerStatusResponse
}

type followerStatusResponse struct {
	Primary             string                    `json:"primary"`
	ClusterID           string                    `json:"-"`
	Sync                string                    `json:"sync"`
	AppliedVersion      int64                     `json:"appliedVersion"`
	PrimaryVersion      int64                     `json:"primaryVersion"`
	LastContactAt       *time.Time                `json:"lastContactAt"`
	LastAppliedAt       *time.Time                `json:"lastAppliedAt"`
	PendingRepositories int                       `json:"pendingRepositories"`
	FailedCopies        int                       `json:"failedCopies"`
	ExtraTags           *int                      `json:"extraTags"`
	InitialCopyDone     bool                      `json:"initialCopyDone"`
	LastError           string                    `json:"lastError,omitempty"`
	CopyError           string                    `json:"copyError,omitempty"`
	Mismatches          []replicationapi.Mismatch `json:"-"`
}

func newFollowerStatusResponse(status *service.FollowerStatus) *followerStatusResponse {
	response := followerStatusResponse(*status)

	return &response
}

type followerHealth struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	AppliedVersion int64  `json:"appliedVersion"`
	Status         string `json:"status"`
}

// apiHealthReady is unauthenticated, so it carries no setting values, logins or follower addresses
func (w *Web) apiHealthReady(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
	defer cancel()

	response := readyResponse{Role: w.role()}

	switch response.Role {
	case rolePrimary:
		response.Ready, response.Checks = w.replication.Ready(ctx)

		if status, err := w.replication.Status(ctx); err == nil {
			response.Cluster, response.StateVersion = status.ClusterID, &status.Version

			for _, f := range status.Followers {
				if f.Connection != service.FollowerConnected {
					continue
				}

				response.Followers = append(response.Followers, followerHealth{Name: f.Name, Version: f.Version, AppliedVersion: f.AppliedVersion, Status: f.Status})
			}
		}

	case roleFollower:
		status, err := w.follower.Status(ctx)

		if err != nil {
			apiJSON(rw, http.StatusServiceUnavailable, readyResponse{Role: roleFollower, Checks: map[string]string{"database": err.Error()}})
			return
		}

		response.Ready, response.Checks = w.follower.Ready(ctx, status)

		follower := newFollowerStatusResponse(status)

		if status.Sync == service.FollowerSetupMismatch {
			follower.LastError = "this follower's setup " + response.Checks["setup"]
		}

		response.followerStatusResponse = follower
		response.Cluster = status.ClusterID

	default:
		response.Checks = map[string]string{"database": "ok"}

		if err := w.health.Ping(ctx); err != nil {
			response.Checks["database"] = err.Error()
		}

		response.Ready = response.Checks["database"] == "ok"
	}

	code := http.StatusOK

	if !response.Ready {
		code = http.StatusServiceUnavailable
	}

	apiJSON(rw, code, response)
}
