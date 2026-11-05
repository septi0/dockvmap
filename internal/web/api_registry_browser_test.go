package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeRegistryBrowser struct {
	result *model.RegistryRepositories
	err    error
}

func (fakeRegistryBrowser) Browsable(context.Context) ([]int64, error) { return []int64{3}, nil }

func (f fakeRegistryBrowser) Repositories(context.Context, int64) (*model.RegistryRepositories, error) {
	return f.result, f.err
}

func TestListRegistryRepositories(t *testing.T) {
	result := &model.RegistryRepositories{Repositories: []model.RegistryRepository{
		{Name: "team/web", Images: []model.Image{{ID: 4, Name: "web"}}},
		{Name: "team/api"},
	}}

	cases := []struct {
		name       string
		browser    fakeRegistryBrowser
		wantStatus int
		wantBody   string
	}{
		{"found", fakeRegistryBrowser{result: result}, http.StatusOK, `{"repositories":[{"name":"team/web","images":[{"id":4,"name":"web"}]},{"name":"team/api","images":[]}],"more":false}`},
		{"unknown registry", fakeRegistryBrowser{}, http.StatusNotFound, "registry not found"},
		{"registry down", fakeRegistryBrowser{err: fmt.Errorf("%w: connection refused", service.ErrUpstreamUnavailable)}, http.StatusBadGateway, "check that the registry is running and its login"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Web{registryBrowser: tc.browser}
			req := httptest.NewRequest(http.MethodGet, "/api/registries/3/repositories", nil)
			req.SetPathValue("id", "3")
			rec := httptest.NewRecorder()

			w.apiListRegistryRepositories(rec, req)

			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("status %d, body %q; want %d containing %q", rec.Code, rec.Body.String(), tc.wantStatus, tc.wantBody)
			}
		})
	}
}
