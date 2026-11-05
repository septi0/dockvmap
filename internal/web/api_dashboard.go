package web

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

const (
	dashboardBehindLimit   = 5
	dashboardUpdatesLimit  = 5
	dashboardActivityLimit = 5
	dashboardIssuesLimit   = 5
)

type dashboardSection[T any] struct {
	Data  *T      `json:"data"`
	Error *string `json:"error"`
}

func dashboardData[T any](data T) dashboardSection[T] {
	return dashboardSection[T]{Data: &data}
}

func dashboardError[T any](message string) dashboardSection[T] {
	return dashboardSection[T]{Error: &message}
}

type dashboardSummaryData struct {
	Images struct {
		Total           int64 `json:"total"`
		UpdateAvailable int64 `json:"updateAvailable"`
		FailedCheck     int64 `json:"failedCheck"`
	} `json:"images"`
}

type dashboardUpdatesData struct {
	Images []imageResponse      `json:"images"`
	Groups []imageGroupResponse `json:"groups"`
	Total  int64                `json:"total"`
}

type dashboardActivityData struct {
	Events []model.ImageEvent `json:"events"`
	Total  int64              `json:"total"`
}

type dashboardIssuesData struct {
	Failures []failureResponse `json:"failures"`
	Total    int64             `json:"total"`
}

type dashboardHostsData struct {
	Online   int                       `json:"online"`
	Total    int                       `json:"total"`
	Outdated int                       `json:"outdated"`
	Behind   []dashboardBehindResponse `json:"behind"`
}

type dashboardBehindResponse struct {
	HostID    int64                 `json:"hostId"`
	HostName  string                `json:"hostName"`
	Container hostContainerResponse `json:"container"`
}

type dashboardResponse struct {
	Summary  dashboardSection[dashboardSummaryData]  `json:"summary"`
	Updates  dashboardSection[dashboardUpdatesData]  `json:"updates"`
	Issues   dashboardSection[dashboardIssuesData]   `json:"issues"`
	Activity dashboardSection[dashboardActivityData] `json:"activity"`
	Metrics  dashboardSection[proxyMetricsResponse]  `json:"metrics"`
	Hosts    dashboardSection[dashboardHostsData]    `json:"hosts"`
}

func (w *Web) apiDashboard(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp dashboardResponse
	var wg sync.WaitGroup

	wg.Add(6)

	go func() { defer wg.Done(); resp.Summary = w.dashboardSummary(ctx) }()
	go func() { defer wg.Done(); resp.Updates = w.dashboardUpdates(ctx) }()
	go func() { defer wg.Done(); resp.Issues = w.dashboardIssues(ctx) }()
	go func() { defer wg.Done(); resp.Activity = w.dashboardActivity(ctx) }()
	go func() { defer wg.Done(); resp.Metrics = w.dashboardMetrics(ctx) }()
	go func() { defer wg.Done(); resp.Hosts = w.dashboardHosts(ctx, w.proxyTarget(r)) }()

	wg.Wait()

	apiJSON(rw, http.StatusOK, resp)
}

func (w *Web) dashboardHosts(ctx context.Context, proxy service.ProxyTarget) dashboardSection[dashboardHostsData] {
	hosts, behind, err := w.hosts.ListWithBehind(ctx, proxy)

	if err != nil {
		return dashboardError[dashboardHostsData]("failed to load hosts")
	}

	data := dashboardHostsData{Total: len(hosts), Behind: []dashboardBehindResponse{}}
	names := make(map[int64]string, len(hosts))

	for _, host := range hosts {
		names[host.ID] = host.Name

		if host.Current() {
			data.Online++
		}

		data.Outdated += host.Rollout.Outdated()
	}

	shown := behind[:min(len(behind), dashboardBehindLimit)]

	for i, container := range newHostContainerResponses(shown) {
		data.Behind = append(data.Behind, dashboardBehindResponse{
			HostID:    shown[i].HostID,
			HostName:  names[shown[i].HostID],
			Container: container,
		})
	}

	return dashboardData(data)
}

func (w *Web) dashboardSummary(ctx context.Context) dashboardSection[dashboardSummaryData] {
	counts, err := w.images.StatusCounts(ctx)

	if err != nil {
		return dashboardError[dashboardSummaryData]("failed to load image counts")
	}

	var data dashboardSummaryData
	data.Images.Total = counts.Total
	data.Images.UpdateAvailable = counts.UpdateAvailable
	data.Images.FailedCheck = counts.FailedCheck

	return dashboardData(data)
}

func (w *Web) dashboardUpdates(ctx context.Context) dashboardSection[dashboardUpdatesData] {
	groupFilters := model.ImageGroupListFilters{
		Pagination:      model.Pagination{Offset: 0, Limit: dashboardUpdatesLimit},
		UpdateAvailable: true,
	}

	groups, err := w.images.ListGroups(ctx, groupFilters)

	if err != nil {
		return dashboardError[dashboardUpdatesData]("failed to load images with updates")
	}

	groupTotal, err := w.images.CountGroups(ctx, groupFilters)

	if err != nil {
		return dashboardError[dashboardUpdatesData]("failed to load images with updates")
	}

	filters := model.ImageListFilters{
		Pagination: model.Pagination{Offset: 0, Limit: dashboardUpdatesLimit - len(groups)},
		Status:     model.ImageStatusUpdateAvailable,
		Ungrouped:  true,
	}

	images := []model.Image{}

	if filters.Limit > 0 {
		images, err = w.images.List(ctx, filters)

		if err != nil {
			return dashboardError[dashboardUpdatesData]("failed to load images with updates")
		}
	}

	total, err := w.images.Count(ctx, filters)

	if err != nil {
		return dashboardError[dashboardUpdatesData]("failed to load images with updates")
	}

	responses := make([]imageResponse, 0, len(images))

	for _, image := range images {
		responses = append(responses, newImageResponse(image))
	}

	return dashboardData(dashboardUpdatesData{
		Images: responses,
		Groups: newImageGroupResponses(groups),
		Total:  total + groupTotal,
	})
}

func (w *Web) dashboardIssues(ctx context.Context) dashboardSection[dashboardIssuesData] {
	filters := model.BackgroundFailureListFilters{
		Pagination: model.Pagination{Offset: 0, Limit: dashboardIssuesLimit},
	}

	failures, err := w.failures.LatestDistinct(ctx, dashboardIssuesLimit)

	if err != nil {
		return dashboardError[dashboardIssuesData]("failed to load recent issues")
	}

	total, err := w.failures.Count(ctx, filters)

	if err != nil {
		return dashboardError[dashboardIssuesData]("failed to load recent issues")
	}

	responses := make([]failureResponse, 0, len(failures))

	for _, failure := range failures {
		responses = append(responses, newFailureResponse(failure))
	}

	return dashboardData(dashboardIssuesData{Failures: responses, Total: total})
}

func (w *Web) dashboardActivity(ctx context.Context) dashboardSection[dashboardActivityData] {
	filters := model.ImageEventListFilters{
		Pagination: model.Pagination{Offset: 0, Limit: dashboardActivityLimit},
	}

	events, err := w.events.List(ctx, filters)

	if err != nil {
		return dashboardError[dashboardActivityData]("failed to load tag activity")
	}

	total, err := w.events.Count(ctx, filters)

	if err != nil {
		return dashboardError[dashboardActivityData]("failed to load tag activity")
	}

	if events == nil {
		events = []model.ImageEvent{}
	}

	return dashboardData(dashboardActivityData{Events: events, Total: total})
}

func (w *Web) dashboardMetrics(ctx context.Context) dashboardSection[proxyMetricsResponse] {
	summary, err := w.proxyMetricsHistory.Summary(ctx)

	if err != nil {
		return dashboardError[proxyMetricsResponse]("failed to load proxy metrics")
	}

	response := proxyMetricsResponse{
		GeneratedAt: time.Now().UTC(),
		Windows:     summary,
	}

	if w.cacheUsage != nil {
		used, max, err := w.cacheUsage.Usage(ctx)

		if err != nil {
			return dashboardError[proxyMetricsResponse]("failed to load cache usage")
		}

		response.Cache = &proxyCacheUsageResponse{UsedBytes: used, MaxBytes: max}
	}

	return dashboardData(response)
}
