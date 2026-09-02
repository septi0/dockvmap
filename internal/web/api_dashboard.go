package web

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

const (
	dashboardUpdatesLimit  = 5
	dashboardActivityLimit = 8
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
	Images []imageResponse `json:"images"`
	Total  int64           `json:"total"`
}

type dashboardResponse struct {
	Summary  dashboardSection[dashboardSummaryData]    `json:"summary"`
	Updates  dashboardSection[dashboardUpdatesData]    `json:"updates"`
	Issues   dashboardSection[[]recentFailureResponse] `json:"issues"`
	Activity dashboardSection[[]model.ImageEvent]      `json:"activity"`
	Metrics  dashboardSection[proxyMetricsResponse]    `json:"metrics"`
}

func (w *Web) apiDashboard(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp dashboardResponse
	var wg sync.WaitGroup

	wg.Add(5)

	go func() { defer wg.Done(); resp.Summary = w.dashboardSummary(ctx) }()
	go func() { defer wg.Done(); resp.Updates = w.dashboardUpdates(ctx) }()
	go func() { defer wg.Done(); resp.Issues = w.dashboardIssues(ctx) }()
	go func() { defer wg.Done(); resp.Activity = w.dashboardActivity(ctx) }()
	go func() { defer wg.Done(); resp.Metrics = w.dashboardMetrics(ctx) }()

	wg.Wait()

	apiJSON(rw, http.StatusOK, resp)
}

func (w *Web) dashboardSummary(ctx context.Context) dashboardSection[dashboardSummaryData] {
	var data dashboardSummaryData

	counts := []struct {
		status model.ImageStatusFilter
		target *int64
	}{
		{"", &data.Images.Total},
		{model.ImageStatusUpdateAvailable, &data.Images.UpdateAvailable},
		{model.ImageStatusFailedCheck, &data.Images.FailedCheck},
	}

	for _, c := range counts {
		count, err := w.images.Count(ctx, model.ImageListFilters{Status: c.status})

		if err != nil {
			return dashboardError[dashboardSummaryData]("failed to load image counts")
		}

		*c.target = count
	}

	return dashboardData(data)
}

func (w *Web) dashboardUpdates(ctx context.Context) dashboardSection[dashboardUpdatesData] {
	filters := model.ImageListFilters{
		Pagination: model.Pagination{Offset: 0, Limit: dashboardUpdatesLimit},
		Status:     model.ImageStatusUpdateAvailable,
	}

	images, err := w.images.List(ctx, filters)

	if err != nil {
		return dashboardError[dashboardUpdatesData]("failed to load images with updates")
	}

	total, err := w.images.Count(ctx, filters)

	if err != nil {
		return dashboardError[dashboardUpdatesData]("failed to load images with updates")
	}

	responses := make([]imageResponse, 0, len(images))

	for _, image := range images {
		responses = append(responses, newImageResponse(image))
	}

	return dashboardData(dashboardUpdatesData{Images: responses, Total: total})
}

func (w *Web) dashboardIssues(ctx context.Context) dashboardSection[[]recentFailureResponse] {
	failures, err := w.failures.Recent(ctx)

	if err != nil {
		return dashboardError[[]recentFailureResponse]("failed to load recent issues")
	}

	responses := make([]recentFailureResponse, 0, len(failures))

	for _, failure := range failures {
		responses = append(responses, newRecentFailureResponse(failure))
	}

	return dashboardData(responses)
}

func (w *Web) dashboardActivity(ctx context.Context) dashboardSection[[]model.ImageEvent] {
	events, err := w.events.List(ctx, 0, dashboardActivityLimit)

	if err != nil {
		return dashboardError[[]model.ImageEvent]("failed to load tag activity")
	}

	if events == nil {
		events = []model.ImageEvent{}
	}

	return dashboardData(events)
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
