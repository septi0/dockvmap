package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
)

type fakeInternalRegistry struct {
	address      string
	repositories []string
	err          error
}

func (f fakeInternalRegistry) Address() string { return f.address }

func (f fakeInternalRegistry) Repositories(context.Context) ([]string, error) {
	return f.repositories, f.err
}

func TestListInternalRepositories(t *testing.T) {
	cases := []struct {
		name       string
		registry   fakeInternalRegistry
		wantStatus int
		wantBody   string
	}{
		{"enabled", fakeInternalRegistry{address: "registry:5000", repositories: []string{"myapp", "team/web"}}, http.StatusOK, `["myapp","team/web"]`},
		{"disabled", fakeInternalRegistry{}, http.StatusNotFound, "not enabled"},
		{"registry down", fakeInternalRegistry{address: "registry:5000", err: errors.New("the internal registry at registry:5000 didn't answer")}, http.StatusBadGateway, "didn't answer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Web{internalRegistry: tc.registry}
			rec := httptest.NewRecorder()

			w.apiListInternalRepositories(rec, httptest.NewRequest(http.MethodGet, "/api/registries/internal/repositories", nil))

			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("status %d, body %q; want %d containing %q", rec.Code, rec.Body.String(), tc.wantStatus, tc.wantBody)
			}
		})
	}
}

func TestProxyAuthStatusReportsTheInternalRegistry(t *testing.T) {
	for _, address := range []string{"", "registry:5000"} {
		cfg := &config.Config{}
		cfg.ProxyAuth.Enabled = true
		w := &Web{cfg: cfg, internalRegistry: fakeInternalRegistry{address: address}}
		rec := httptest.NewRecorder()

		w.apiProxyAuthStatus(rec, httptest.NewRequest(http.MethodGet, "/api/proxy-auth", nil))

		var status map[string]bool

		if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
			t.Fatalf("body %q: %v", rec.Body.String(), err)
		}

		if !status["enabled"] || status["internalRegistry"] != (address != "") {
			t.Errorf("address %q: status = %v, want enabled and internalRegistry %v", address, status, address != "")
		}
	}
}
