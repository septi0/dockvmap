package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
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
