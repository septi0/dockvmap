package web

import (
	"fmt"
	"net/http"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type listAuditLogsResponse struct {
	AuditLogs []model.AuditLog `json:"auditLogs"`
	Total     int64            `json:"total"`
}

const maxAuditLogsLimit = 200

func parseAuditLogListFilters(r *http.Request) (model.AuditLogListFilters, error) {
	pagination, err := parsePagination(r, maxAuditLogsLimit)

	if err != nil {
		return model.AuditLogListFilters{}, err
	}

	auditType := r.URL.Query().Get("type")

	if auditType != "" && !service.IsValidAuditType(auditType) {
		return model.AuditLogListFilters{}, fmt.Errorf("type must be one of the known audit event types")
	}

	since, err := parseTimeParam(r, "since")

	if err != nil {
		return model.AuditLogListFilters{}, err
	}

	until, err := parseTimeParam(r, "until")

	if err != nil {
		return model.AuditLogListFilters{}, err
	}

	return model.AuditLogListFilters{
		Pagination: pagination,
		Type:       auditType,
		Since:      since,
		Until:      until,
	}, nil
}

func (w *Web) apiListAuditLogs(rw http.ResponseWriter, r *http.Request) {
	filters, err := parseAuditLogListFilters(r)

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())
		return
	}

	logs, err := w.audit.List(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	total, err := w.audit.Count(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	apiJSON(rw, http.StatusOK, listAuditLogsResponse{
		AuditLogs: logs,
		Total:     total,
	})
}
