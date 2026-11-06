package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeImages struct {
	imageService
	groups []model.ImageGroup
	list   []model.Image
	err    error
}

func (f fakeImages) List(context.Context, model.ImageListFilters) ([]model.Image, error) {
	return f.list, f.err
}

func (f fakeImages) Count(context.Context, model.ImageListFilters) (int64, error) {
	return int64(len(f.list)), f.err
}

func (f fakeImages) ListGroups(context.Context, model.ImageGroupListFilters) ([]model.ImageGroup, error) {
	return f.groups, nil
}

func (f fakeImages) CountGroups(context.Context, model.ImageGroupListFilters) (int64, error) {
	return int64(len(f.groups)), nil
}

func (f fakeImages) StatusCounts(context.Context) (model.ImageStatusCounts, error) {
	return model.ImageStatusCounts{Total: int64(len(f.list))}, f.err
}

type fakeHosts struct {
	hostService
	list   []model.Host
	behind []model.HostContainer
}

func (f fakeHosts) ListWithBehind(context.Context, service.ProxyTarget) ([]model.Host, []model.HostContainer, error) {
	return f.list, f.behind, nil
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

func (f fakeFailures) LatestDistinct(context.Context, int) ([]service.Failure, error) {
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
	dockerFailure := "cannot read Docker"
	behind := make([]model.HostContainer, 6)

	for i := range behind {
		behind[i] = model.HostContainer{HostID: 1, Name: fmt.Sprintf("web-%d", i)}
	}

	w := &Web{
		images:              fakeImages{list: []model.Image{{ID: 1, Name: "app"}}},
		events:              fakeEvents{list: []model.ImageEvent{{ID: 7}}},
		failures:            fakeFailures{list: []service.Failure{{Error: "boom"}}},
		proxyMetricsHistory: fakeMetrics{},
		hosts: fakeHosts{
			list:   []model.Host{{ID: 1, Name: "nas", Online: true, Rollout: model.Rollout{UpToDate: 2, Total: 5}}, {ID: 2, Online: true, DockerError: &dockerFailure}, {ID: 3}},
			behind: behind,
		},
		cfg: &config.Config{ProxyServerListen: ":5000"},
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

	if resp.Issues.Data == nil || resp.Issues.Data.Distinct != 1 || len(resp.Issues.Data.Failures) != 1 {
		t.Errorf("issues = %+v, want 1 distinct failure", resp.Issues.Data)
	}

	if resp.Activity.Data == nil || len(resp.Activity.Data.Events) != 1 {
		t.Errorf("activity = %+v, want 1 event", resp.Activity.Data)
	}

	if resp.Hosts.Data == nil || resp.Hosts.Data.Online != 1 || resp.Hosts.Data.Total != 3 || resp.Hosts.Data.Outdated != 3 {
		t.Errorf("hosts = %+v, want 1 of 3 reporting, with a Docker failure not counted, and 3 outdated containers", resp.Hosts.Data)
	}

	if shown := resp.Hosts.Data.Behind; len(shown) != 6 || shown[0].HostName != "nas" || shown[0].Container.Name != "web-0" {
		t.Errorf("behind = %+v, want all 6 with their host name", shown)
	}

	if attention := resp.Hosts.Data.Attention; len(attention) != 2 || attention[0].ID != 2 || attention[1].ID != 3 {
		t.Errorf("attention = %+v, want the host that can't read Docker and the offline one", attention)
	}

	if resp.Hosts.Data.TrackedContainers != 5 || resp.Hosts.Data.TrackedHosts != 1 {
		t.Errorf("tracked = %d containers on %d hosts, want 5 on 1", resp.Hosts.Data.TrackedContainers, resp.Hosts.Data.TrackedHosts)
	}
}

func TestDashboardIsolatesSectionFailure(t *testing.T) {
	w := &Web{
		images:              fakeImages{list: []model.Image{{ID: 1, Name: "app"}}},
		events:              fakeEvents{list: []model.ImageEvent{{ID: 7}}},
		failures:            fakeFailures{err: errors.New("db is down")},
		proxyMetricsHistory: fakeMetrics{},
		hosts:               fakeHosts{list: []model.Host{{ID: 1, Online: true}, {ID: 2}}},
		cfg:                 &config.Config{ProxyServerListen: ":5000"},
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
		hosts:               fakeHosts{list: []model.Host{{ID: 1, Online: true}, {ID: 2}}},
		cfg:                 &config.Config{ProxyServerListen: ":5000"},
	}

	resp := dashboardFor(t, w)

	if resp.Summary.Error == nil || resp.Updates.Error == nil {
		t.Fatal("image-backed sections reported no error, want one on each")
	}

	if resp.Activity.Data == nil || resp.Metrics.Data == nil {
		t.Error("non-image sections did not survive the image failure")
	}
}

func TestDashboardUpdatesCountsGroupsAsOneRow(t *testing.T) {
	w := &Web{images: fakeImages{
		list:   []model.Image{{ID: 1, Name: "app"}},
		groups: []model.ImageGroup{{ID: 1, Name: "stack"}, {ID: 2, Name: "other"}},
	}}

	section := w.dashboardUpdates(context.Background())

	if section.Error != nil {
		t.Fatalf("error = %v", *section.Error)
	}

	if len(section.Data.Groups) != 2 || len(section.Data.Images) != 1 || section.Data.Total != 3 {
		t.Fatalf("data = %+v", section.Data)
	}
}

func TestDashboardActivityShowsANewUpgradeTagOnce(t *testing.T) {
	tags := func(t ...string) model.TagsEventData { return model.TagsEventData{Tags: t} }

	w := &Web{
		images:              fakeImages{},
		failures:            fakeFailures{},
		proxyMetricsHistory: fakeMetrics{},
		hosts:               fakeHosts{},
		cfg:                 &config.Config{ProxyServerListen: ":5000"},
		events: fakeEvents{list: []model.ImageEvent{
			{ID: 6, ImageID: 1, Type: service.EventTypeUpgradeAvailable, Data: tags("v2")},
			{ID: 5, ImageID: 1, Type: service.EventTypeTagAdded, Data: tags("v2")},
			{ID: 4, ImageID: 2, Type: service.EventTypeTagAdded, Data: tags("v9", "latest-rc")},
			{ID: 3, ImageID: 2, Type: service.EventTypeUpgradeAvailable, Data: tags("v9")},
			{ID: 2, ImageID: 3, Type: service.EventTypeTagRemoved, Data: tags("old")},
		}},
	}

	resp := dashboardFor(t, w)

	var ids []int64

	for _, event := range resp.Activity.Data.Events {
		ids = append(ids, event.ID)
	}

	if !slices.Equal(ids, []int64{6, 4, 3, 2}) || resp.Activity.Data.Total != 5 {
		t.Errorf("events = %v (total %d), want the tag added with its upgrade hidden, others kept", ids, resp.Activity.Data.Total)
	}
}
