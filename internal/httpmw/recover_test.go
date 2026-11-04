package httpmw

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/septi0/dockvmap/internal/ipmatch"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &logs
}

func TestRecoverLogsAPanicAndAnswers500(t *testing.T) {
	logs := captureLogs(t)

	handler := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v2/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}

	if !strings.Contains(logs.String(), "panic serving request") {
		t.Errorf("panic not logged: %q", logs.String())
	}
}

func TestRecoverPassesAbortHandlerOn(t *testing.T) {
	logs := captureLogs(t)

	handler := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", recovered)
		}

		if strings.Contains(logs.String(), "panic serving request") {
			t.Errorf("an aborted request was logged as a panic: %q", logs.String())
		}
	}()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v2/", nil))
}

func TestInterimStatusesDontCountAsTheResponse(t *testing.T) {
	logs := captureLogs(t)

	handler := AccessLog("proxy", ipmatch.Set{}, Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusContinue)
		panic("boom")
	})))

	server := httptest.NewServer(handler)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPatch, server.URL+"/v2/team/app/blobs/uploads/1", strings.NewReader("layer"))
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status after a 100 and a panic = %d, want 500", resp.StatusCode)
	}

	if !strings.Contains(logs.String(), "status=500") {
		t.Errorf("access log doesn't show the final status: %q", logs.String())
	}
}
