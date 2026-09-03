package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeImages struct {
	imageService
	list []model.Image
	err  error
}

func (f fakeImages) List(context.Context, model.ImageListFilters) ([]model.Image, error) {
	return f.list, f.err
}

func (f fakeImages) Count(context.Context, model.ImageListFilters) (int64, error) {
	return int64(len(f.list)), f.err
}

func (f fakeImages) StatusCounts(context.Context) (model.ImageStatusCounts, error) {
	return model.ImageStatusCounts{Total: int64(len(f.list))}, f.err
}

type fakeEvents struct {
	list []model.ImageEvent
	err  error
}

func (f fakeEvents) List(context.Context, model.ImageEventListFilters) ([]model.ImageEvent, error) {
	return f.list, f.err
}

func (f fakeEvents) Count(context.Context, model.ImageEventListFilters) (int64, error) {
	return int64(len(f.list)), f.err
}

type fakeFailures struct {
	list []service.Failure
	err  error
}

func (f fakeFailures) List(context.Context, model.BackgroundFailureListFilters) ([]service.Failure, error) {
	return f.list, f.err
}

func (f fakeFailures) Count(context.Context, model.BackgroundFailureListFilters) (int64, error) {
	return int64(len(f.list)), f.err
}

type fakeMetrics struct {
	err error
}

func (f fakeMetrics) Summary(context.Context) (model.ProxyMetricsSummary, error) {
	return model.ProxyMetricsSummary{}, f.err
}

func dashboardFor(t *testing.T, w *Web) dashboardResponse {
	t.Helper()

	rec := httptest.NewRecorder()
	w.apiDashboard(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp dashboardResponse

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	return resp
}

func TestDashboardAllSectionsSucceed(t *testing.T) {
	w := &Web{
		images:              fakeImages{list: []model.Image{{ID: 1, Name: "app"}}},
		events:              fakeEvents{list: []model.ImageEvent{{ID: 7}}},
		failures:            fakeFailures{list: []service.Failure{{Error: "boom"}}},
		proxyMetricsHistory: fakeMetrics{},
	}

	resp := dashboardFor(t, w)

	sections := map[string]*string{
		"summary":  resp.Summary.Error,
		"updates":  resp.Updates.Error,
		"issues":   resp.Issues.Error,
		"activity": resp.Activity.Error,
		"metrics":  resp.Metrics.Error,
	}

	for name, err := range sections {
		if err != nil {
			t.Errorf("section %q reported error %q, want none", name, *err)
		}
	}

	if resp.Summary.Data == nil || resp.Summary.Data.Images.Total != 1 {
		t.Errorf("summary total = %+v, want 1", resp.Summary.Data)
	}

	if resp.Updates.Data == nil || len(resp.Updates.Data.Images) != 1 {
		t.Errorf("updates images = %+v, want 1", resp.Updates.Data)
	}

	if resp.Activity.Data == nil || len(resp.Activity.Data.Events) != 1 {
		t.Errorf("activity = %+v, want 1 event", resp.Activity.Data)
	}
}

func TestDashboardIsolatesSectionFailure(t *testing.T) {
	w := &Web{
		images:              fakeImages{list: []model.Image{{ID: 1, Name: "app"}}},
		events:              fakeEvents{list: []model.ImageEvent{{ID: 7}}},
		failures:            fakeFailures{err: errors.New("db is down")},
		proxyMetricsHistory: fakeMetrics{},
	}

	resp := dashboardFor(t, w)

	if resp.Issues.Error == nil {
		t.Fatal("issues section reported no error, want one")
	}

	if resp.Issues.Data != nil {
		t.Errorf("issues data = %+v, want nil alongside an error", resp.Issues.Data)
	}

	if resp.Summary.Data == nil || resp.Summary.Error != nil {
		t.Error("summary section did not survive the issues failure")
	}

	if resp.Activity.Data == nil || resp.Activity.Error != nil {
		t.Error("activity section did not survive the issues failure")
	}

	if resp.Metrics.Data == nil || resp.Metrics.Error != nil {
		t.Error("metrics section did not survive the issues failure")
	}
}

func TestDashboardImageFailureIsolatedFromRest(t *testing.T) {
	w := &Web{
		images:              fakeImages{err: errors.New("db is down")},
		events:              fakeEvents{list: []model.ImageEvent{{ID: 7}}},
		failures:            fakeFailures{},
		proxyMetricsHistory: fakeMetrics{},
	}

	resp := dashboardFor(t, w)

	if resp.Summary.Error == nil || resp.Updates.Error == nil {
		t.Fatal("image-backed sections reported no error, want one on each")
	}

	if resp.Activity.Data == nil || resp.Metrics.Data == nil {
		t.Error("non-image sections did not survive the image failure")
	}
}
