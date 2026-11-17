package replication

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/ocicopy"
	"github.com/septi0/dockvmap/internal/replicationapi"
	"github.com/septi0/dockvmap/internal/service"
)

type fakePrimary struct {
	response *replicationapi.StateResponse
	conflict *replicationapi.Conflict
	seen     string
	request  service.StateRequest
	pulls    []replicationapi.PullReport
	authErr  error
}

func (f *fakePrimary) Authenticate(ctx context.Context, secret string) (*model.ReplicationToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if f.authErr != nil {
		return nil, f.authErr
	}

	if secret != "good" {
		return nil, nil
	}

	return &model.ReplicationToken{ID: 1, Label: "standby-1"}, nil
}

func (f *fakePrimary) State(_ context.Context, req service.StateRequest) (*replicationapi.StateResponse, *replicationapi.Conflict, error) {
	f.request = req

	return f.response, f.conflict, nil
}

func (f *fakePrimary) Changes(context.Context, string, int64) (*replicationapi.ChangesResponse, error) {
	return &replicationapi.ChangesResponse{Complete: true}, nil
}

func (f *fakePrimary) RecordPulls(_ context.Context, report replicationapi.PullReport) error {
	f.pulls = append(f.pulls, report)

	return nil
}

func (f *fakePrimary) SeeFollower(token *model.ReplicationToken, _, version, status string, _ int64) {
	f.seen = token.Label + " " + version + " " + status
}

func request(t *testing.T, h http.Handler, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, nil)

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("User-Agent", "dockvmap/1.14.0")
	req.Header.Set(replicationapi.FollowerStatusHeader, "syncing")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return rec
}

func TestStateNeedsAValidToken(t *testing.T) {
	h := NewHandler(&fakePrimary{}, nil, ipmatch.Set{})

	for _, token := range []string{"", "bad"} {
		if rec := request(t, h, http.MethodGet, replicationapi.StatePath, token); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401", token, rec.Code)
		}
	}
}

func TestStateAnswers(t *testing.T) {
	primary := &fakePrimary{}
	h := NewHandler(primary, nil, ipmatch.Set{})

	rec := request(t, h, http.MethodGet, replicationapi.StatePath+"?version=4&tag=ab&cluster=c1", "good")

	if rec.Code != http.StatusNotModified {
		t.Errorf("unchanged state: status = %d, want 304", rec.Code)
	}

	if primary.request != (service.StateRequest{Version: 4, Tag: "ab", Cluster: "c1", HasVersion: true}) {
		t.Errorf("request = %+v, want version 4, tag ab, cluster c1", primary.request)
	}

	if primary.seen != "standby-1 1.14.0 syncing" {
		t.Errorf("seen follower = %q, want its label, version and status", primary.seen)
	}

	primary.conflict = &replicationapi.Conflict{Reason: replicationapi.ConflictDiverged, PrimaryVersion: 2}

	if rec := request(t, h, http.MethodGet, replicationapi.StatePath+"?version=4", "good"); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "diverged") {
		t.Errorf("conflict: status = %d, body %s, want 409 with the reason", rec.Code, rec.Body)
	}

	primary.conflict = nil
	primary.response = &replicationapi.StateResponse{APIVersion: 1}

	if rec := request(t, h, http.MethodGet, replicationapi.StatePath+"?full=1", "good"); rec.Code != http.StatusOK || !primary.request.Full {
		t.Errorf("snapshot: status = %d, full = %v, want 200 for a full request", rec.Code, primary.request.Full)
	}

	if rec := request(t, h, http.MethodGet, replicationapi.StatePath+"?version=x", "good"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad version: status = %d, want 400", rec.Code)
	}
}

func TestRegistryReadIsReadOnly(t *testing.T) {
	var got *http.Request

	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusOK)
	}))
	defer registry.Close()

	h := NewHandler(&fakePrimary{}, &ocicopy.Registry{BaseURL: registry.URL, Client: registry.Client()}, ipmatch.Set{})

	if rec := request(t, h, http.MethodGet, replicationapi.RegistryPrefix+"/v2/team/web/tags/list", "good"); rec.Code != http.StatusOK {
		t.Fatalf("read: status = %d, want 200", rec.Code)
	}

	if got.URL.Path != "/v2/team/web/tags/list" || got.Header.Get("Authorization") != "" {
		t.Errorf("registry got %s with Authorization %q, want the plain registry path and no token", got.URL.Path, got.Header.Get("Authorization"))
	}

	got = nil

	for method, target := range map[string]string{
		http.MethodPost:   replicationapi.RegistryPrefix + "/v2/team/web/blobs/uploads/",
		http.MethodDelete: replicationapi.RegistryPrefix + "/v2/team/web/manifests/v1",
		http.MethodGet:    replicationapi.RegistryPrefix + "/debug/vars",
	} {
		rec := request(t, h, method, target, "good")

		if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want it refused", method, target, rec.Code)
		}
	}

	if got != nil {
		t.Errorf("the registry received %s %s, want nothing but reads of /v2/", got.Method, got.URL.Path)
	}

	if rec := request(t, NewHandler(&fakePrimary{}, nil, ipmatch.Set{}), http.MethodGet, replicationapi.RegistryPrefix+"/v2/", "good"); rec.Code != http.StatusNotFound {
		t.Errorf("without an internal registry: status = %d, want 404", rec.Code)
	}
}

func TestStateRefusesAddressesOutsideTheAllowedList(t *testing.T) {
	allowed, _ := ipmatch.Parse([]string{"10.0.0.12", "10.0.1.0/24"})
	h := NewHandler(&fakePrimary{}, nil, allowed)

	for remote, want := range map[string]int{
		"10.0.0.12:40000":   http.StatusNotModified,
		"10.0.1.7:40000":    http.StatusNotModified,
		"192.168.1.5:40000": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, replicationapi.StatePath+"?version=4", nil)
		req.RemoteAddr = remote
		req.Header.Set("Authorization", "Bearer good")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != want {
			t.Errorf("from %s: status = %d, want %d", remote, rec.Code, want)
		}

		if want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "replication isn't allowed from 192.168.1.5; add it to replication.allowed_ips on the primary") {
			t.Errorf("from %s: body %s, want it to name the address and the setting", remote, rec.Body)
		}
	}
}

func TestPullReportNeedsATokenAndReachesThePrimary(t *testing.T) {
	primary := &fakePrimary{}
	h := NewHandler(primary, nil, ipmatch.Set{})

	post := func(token, body string) int {
		req := httptest.NewRequest(http.MethodPost, replicationapi.PullsPath, strings.NewReader(body))

		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		return rec.Code
	}

	if code := post("", `{}`); code != http.StatusUnauthorized {
		t.Errorf("without a token: status = %d, want 401", code)
	}

	if code := post("good", `not json`); code != http.StatusBadRequest {
		t.Errorf("bad body: status = %d, want 400", code)
	}

	if code := post("good", `{"images":[{"imageId":3,"pulledAt":"2026-11-14T10:00:00Z","pulledTag":"v1"}]}`); code != http.StatusNoContent || len(primary.pulls) != 1 || primary.pulls[0].Images[0].ImageID != 3 {
		t.Errorf("report: status = %d, pulls %+v, want 204 and the image passed on", code, primary.pulls)
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &logs
}

func TestARequestCancelledByShutdownAnswers503WithoutAnError(t *testing.T) {
	logs := captureLogs(t)
	h := NewHandler(&fakePrimary{}, nil, ipmatch.Set{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, replicationapi.StatePath, nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 so the follower retries", rec.Code)
	}

	if strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("logs = %s, want no ERROR for a request cancelled by shutdown", logs)
	}
}

func TestAFailedTokenCheckAnswers500AndLogsAnError(t *testing.T) {
	logs := captureLogs(t)
	h := NewHandler(&fakePrimary{authErr: errors.New("database is locked")}, nil, ipmatch.Set{})

	if rec := request(t, h, http.MethodGet, replicationapi.StatePath, "good"); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Errorf("logs = %s, want an ERROR for a real failure", logs)
	}
}
