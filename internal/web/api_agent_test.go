package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeAgentHosts struct {
	hostService
	reports []agentapi.Report
	err     error
}

func (f *fakeAgentHosts) Authenticate(_ context.Context, token string) (int64, bool, error) {
	return 7, token == "good", nil
}

func (f *fakeAgentHosts) Report(_ context.Context, hostID int64, report agentapi.Report) error {
	if hostID != 7 {
		return service.ErrHostNotFound
	}

	f.reports = append(f.reports, report)

	return f.err
}

func (f *fakeAgentHosts) Heartbeat(_ context.Context, hostID int64, heartbeat agentapi.Heartbeat) error {
	if hostID != 7 {
		return service.ErrHostNotFound
	}

	return f.err
}

func agentHandler(hosts *fakeAgentHosts, cfg *config.Config) http.Handler {
	w := &Web{hosts: hosts, cfg: cfg}
	mux := http.NewServeMux()
	w.registerAgentRoutes(mux)

	return http.StripPrefix("/api/agent", w.requireAgent(mux))
}

func agentRequest(method, path, token, body string) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "dockvmap.example.com:8443"

	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}

	return r
}

func TestAgentAPIRequiresAValidToken(t *testing.T) {
	handler := agentHandler(&fakeAgentHosts{}, &config.Config{ProxyServerListen: ":5000"})

	for _, token := range []string{"", "bad"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, agentRequest(http.MethodGet, agentapi.ConfigPath, token, ""))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d", token, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	r := agentRequest(http.MethodGet, agentapi.ConfigPath, "", "")
	r.Header.Set("Authorization", "Basic Z29vZA==")
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("basic auth: status = %d", rec.Code)
	}
}

func TestAgentConfigUsesTheProxyAddress(t *testing.T) {
	cases := map[string]struct {
		cfg  config.Config
		want string
	}{
		"configured host": {config.Config{ProxyPublicHost: "docker-registry.uscrib.ro:5050", ProxyServerListen: ":5000"}, "docker-registry.uscrib.ro:5050"},
		"request host":    {config.Config{ProxyServerListen: ":5000"}, "dockvmap.example.com:5000"},
	}

	for name, tc := range cases {
		rec := httptest.NewRecorder()
		agentHandler(&fakeAgentHosts{}, &tc.cfg).ServeHTTP(rec, agentRequest(http.MethodGet, agentapi.ConfigPath, "good", ""))

		var got agentapi.Config

		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, err %v", name, rec.Code, err)
		}

		if got.ProxyHost != tc.want {
			t.Errorf("%s: config = %+v, want proxy %q", name, got, tc.want)
		}
	}
}

func TestAgentReportResponses(t *testing.T) {
	cfg := &config.Config{ProxyServerListen: ":5000"}

	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"accepted", `{"protocolVersion":1,"containers":[],"futureField":true}`, nil, http.StatusOK},
		{"invalid JSON", `{"protocolVersion":`, nil, http.StatusBadRequest},
		{"too large", `{"agentVersion":"` + strings.Repeat("a", maxAgentReportBytes) + `"}`, nil, http.StatusRequestEntityTooLarge},
		{"rejected report", `{}`, service.ErrInvalidAgentReport, http.StatusBadRequest},
		{"unsupported protocol", `{}`, service.ErrUnsupportedAgentProtocol, http.StatusBadRequest},
		{"host deleted", `{}`, service.ErrHostNotFound, http.StatusUnauthorized},
		{"storage failure", `{}`, context.DeadlineExceeded, http.StatusInternalServerError},
	}

	for _, tc := range cases {
		hosts := &fakeAgentHosts{err: tc.err}
		rec := httptest.NewRecorder()
		agentHandler(hosts, cfg).ServeHTTP(rec, agentRequest(http.MethodPost, agentapi.ReportPath, "good", tc.body))

		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	rec := httptest.NewRecorder()
	agentHandler(&fakeAgentHosts{}, cfg).ServeHTTP(rec, agentRequest(http.MethodGet, agentapi.ReportPath, "good", ""))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET report: status = %d", rec.Code)
	}
}

func TestAgentRoutesBypassSessionsButHostRoutesDoNot(t *testing.T) {
	handler, err := New(Dependencies{
		Config: &config.Config{ProxyServerListen: ":5000"},
		Hosts:  &fakeAgentHosts{},
	})

	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path, token string
		want        int
		body        string
	}{
		{agentapi.ConfigPath, "good", http.StatusOK, "proxyHost"},
		{agentapi.ConfigPath, "", http.StatusUnauthorized, "agent token required"},
		{"/api/hosts", "good", http.StatusUnauthorized, "authentication required"},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, agentRequest(http.MethodGet, tc.path, tc.token, ""))

		if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s with token %q: %d %s", tc.path, tc.token, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentHeartbeatResponses(t *testing.T) {
	cfg := &config.Config{ProxyServerListen: ":5000"}

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unchanged", nil, http.StatusOK},
		{"full report needed", service.ErrFullReportNeeded, agentapi.FullReportNeededStatus},
		{"unsupported protocol", service.ErrUnsupportedAgentProtocol, http.StatusBadRequest},
		{"host deleted", service.ErrHostNotFound, http.StatusUnauthorized},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		agentHandler(&fakeAgentHosts{err: tc.err}, cfg).ServeHTTP(rec, agentRequest(http.MethodPost, agentapi.HeartbeatPath, "good", `{"protocolVersion":1,"fingerprint":"abc"}`))

		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
}
