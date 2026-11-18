package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeFollower struct {
	status service.FollowerStatus
	ready  bool
	checks map[string]string
}

func (f fakeFollower) Status(context.Context) (*service.FollowerStatus, error) {
	status := f.status
	return &status, nil
}

func (f fakeFollower) Ready(context.Context, *service.FollowerStatus) (bool, map[string]string) {
	return f.ready, f.checks
}

func TestFollowerServesOnlyHealthAndAPointerToThePrimary(t *testing.T) {
	handler, err := New(Dependencies{
		Follower: fakeFollower{ready: true, checks: map[string]string{"database": "ok"}},
		Config:   &config.Config{Follower: config.FollowerConfig{Enabled: true, Primary: "10.0.0.11:9003"}},
	})

	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/health/ready", http.StatusOK},
		{http.MethodGet, "/api/images", http.StatusNotFound},
		{http.MethodPost, "/api/setup", http.StatusNotFound},
		{http.MethodPost, "/api/login", http.StatusNotFound},
		{http.MethodGet, "/assets/index.js", http.StatusNotFound},
	}

	for _, c := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))

		if rec.Code != c.want {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.want)
		}

		if c.path != "/health/ready" && !strings.Contains(rec.Body.String(), "read-only follower of 10.0.0.11:9003") {
			t.Errorf("%s %s: body %q, want it to point to the primary", c.method, c.path, rec.Body)
		}
	}
}

func TestHealthReadyOnAFollowerHidesSettingValues(t *testing.T) {
	w := &Web{follower: fakeFollower{
		status: service.FollowerStatus{
			Primary:        "10.0.0.11:9003",
			ClusterID:      "c1",
			Sync:           service.FollowerSetupMismatch,
			AppliedVersion: 7,
			LastError:      `Sync stopped: virtual_tag is "stable" on the primary and "current" here.`,
			Mismatches:     []replicationapi.Mismatch{{Setting: "virtual_tag", Primary: "stable", Follower: "current"}},
		},
		checks: map[string]string{"database": "ok", "setup": "differs from the primary: virtual_tag"},
	}}

	rec := httptest.NewRecorder()
	w.apiHealthReady(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	if strings.Contains(rec.Body.String(), "stable") {
		t.Errorf("body %s shows a setting value, want only setting names", rec.Body)
	}

	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)

	if body["role"] != "follower" || body["cluster"] != "c1" || body["sync"] != service.FollowerSetupMismatch || body["primary"] != "10.0.0.11:9003" {
		t.Errorf("body = %v, want role, cluster, sync and primary", body)
	}
}

type fakePrimary struct {
	followers []service.SeenFollower
}

func (p fakePrimary) Status(context.Context) (*service.ReplicationStatus, error) {
	return &service.ReplicationStatus{ClusterID: "c1", Version: 5, Followers: p.followers}, nil
}

func (fakePrimary) Ready(context.Context) (bool, map[string]string) {
	return true, map[string]string{"database": "ok"}
}

func (fakePrimary) ListTokens(context.Context) ([]model.ReplicationToken, error) { return nil, nil }

func (fakePrimary) CreateToken(context.Context, string) (int64, string, error) { return 0, "", nil }

func (fakePrimary) DeleteToken(context.Context, int64) (bool, error) { return false, nil }

func (p fakePrimary) Followers(context.Context) ([]service.SeenFollower, error) {
	return p.followers, nil
}

func (fakePrimary) RenameFollower(context.Context, int64, string) (bool, error) { return false, nil }

func (fakePrimary) RemoveFollower(context.Context, int64) (bool, error) { return false, nil }

func TestHealthReadyOnAPrimaryNeverShowsFollowerAddresses(t *testing.T) {
	w := &Web{replication: fakePrimary{followers: []service.SeenFollower{
		{Name: "DC standby", Address: "10.0.0.12", Connection: service.FollowerConnected},
		{Address: "10.0.0.13", Connection: service.FollowerConnected},
		{Name: "site-b", Address: "10.0.0.14", Connection: service.FollowerMissing},
	}}}

	rec := httptest.NewRecorder()
	w.apiHealthReady(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if strings.Contains(rec.Body.String(), "10.0.0.") {
		t.Errorf("body %s shows a follower address, want none on an endpoint without auth", rec.Body)
	}

	var body struct {
		Followers []struct{ Name string } `json:"followers"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)

	if len(body.Followers) != 2 || body.Followers[0].Name != "DC standby" || body.Followers[1].Name != "" {
		t.Errorf("followers = %+v, want the two connected ones, named only by the name given on the primary", body.Followers)
	}
}
