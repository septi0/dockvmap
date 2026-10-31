package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

const maxImageGroupsLimit = 200

const (
	imageGroupStatusUpdateAvailable = "updateAvailable"
	imageGroupStatusPinned          = "pinned"
)

type listImageGroupsResponse struct {
	Groups []imageGroupResponse `json:"groups"`
	Total  int64                `json:"total"`
}

func isGroupTagError(err error) bool {
	var groupTagErr *service.GroupTagUnavailableError

	return errors.As(err, &groupTagErr)
}

func parseImageGroupFilters(r *http.Request) (model.ImageGroupListFilters, error) {
	pagination, err := parsePagination(r, maxImageGroupsLimit)

	if err != nil {
		return model.ImageGroupListFilters{}, err
	}

	status := r.URL.Query().Get("status")

	if status != "" && status != imageGroupStatusUpdateAvailable && status != imageGroupStatusPinned {
		return model.ImageGroupListFilters{}, fmt.Errorf("status must be one of: %s, %s", imageGroupStatusUpdateAvailable, imageGroupStatusPinned)
	}

	return model.ImageGroupListFilters{
		Pagination:      pagination,
		Search:          strings.TrimSpace(r.URL.Query().Get("search")),
		UpdateAvailable: status == imageGroupStatusUpdateAvailable,
		Pinned:          status == imageGroupStatusPinned,
	}, nil
}

func parseImageGroupID(rw http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image group id must be a valid integer")

		return 0, false
	}

	return id, true
}

func apiImageGroupError(rw http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrImageGroupNotFound):
		apiError(rw, http.StatusNotFound, "image group not found")

	case errors.Is(err, service.ErrImageNotFound):
		apiError(rw, http.StatusNotFound, "image not found")

	case errors.Is(err, service.ErrImageGroupAlreadyExists):
		apiError(rw, http.StatusConflict, "image group name already exists")

	default:
		apiServiceError(rw, err)
	}
}

func (w *Web) apiListImageGroups(rw http.ResponseWriter, r *http.Request) {
	filters, err := parseImageGroupFilters(r)

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())

		return
	}

	groups, err := w.images.ListGroups(r.Context(), filters)

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	total, err := w.images.CountGroups(r.Context(), filters)

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	for i := range groups {
		w.proxyUsage.ApplyToImages(groups[i].Images)
	}

	apiJSON(rw, http.StatusOK, listImageGroupsResponse{
		Groups: newImageGroupResponses(groups),
		Total:  total,
	})
}

func (w *Web) apiCreateImageGroup(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[imageGroupRequest](rw, r)
	if !ok {
		return
	}

	id, err := w.images.CreateGroup(r.Context(), service.ImageGroupInput{
		Name:     request.Name,
		ImageIDs: request.ImageIDs,
		Tag:      request.Tag,
	})

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	apiJSON(rw, http.StatusCreated, map[string]any{"status": "created", "id": id})
}

func (w *Web) apiGetImageGroup(rw http.ResponseWriter, r *http.Request) {
	id, ok := parseImageGroupID(rw, r)
	if !ok {
		return
	}

	group, err := w.images.GetGroup(r.Context(), id)

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	w.proxyUsage.ApplyToImages(group.Images)

	apiJSON(rw, http.StatusOK, newImageGroupResponse(*group))
}

func (w *Web) apiUpdateImageGroup(rw http.ResponseWriter, r *http.Request) {
	id, ok := parseImageGroupID(rw, r)
	if !ok {
		return
	}

	request, ok := decodeJSON[imageGroupRequest](rw, r)
	if !ok {
		return
	}

	err := w.images.UpdateGroup(r.Context(), id, service.ImageGroupInput{
		Name:     request.Name,
		ImageIDs: request.ImageIDs,
		Tag:      request.Tag,
	})

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) apiDeleteImageGroup(rw http.ResponseWriter, r *http.Request) {
	id, ok := parseImageGroupID(rw, r)
	if !ok {
		return
	}

	if err := w.images.DeleteGroup(r.Context(), id); err != nil {
		apiImageGroupError(rw, err)

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func (w *Web) apiGetCommonTags(rw http.ResponseWriter, r *http.Request) {
	ids, err := parseImageIDs(r.URL.Query().Get("imageIds"))

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())

		return
	}

	tags, err := w.images.CommonTags(r.Context(), ids)

	if err != nil {
		apiImageGroupError(rw, err)

		return
	}

	apiJSON(rw, http.StatusOK, newImageTagsResponse(tags, ""))
}

func parseImageIDs(value string) ([]int64, error) {
	parts := strings.Split(value, ",")

	if value == "" || len(parts) > maxImageGroupsLimit {
		return nil, fmt.Errorf("imageIds must list between 1 and %d image ids", maxImageGroupsLimit)
	}

	ids := make([]int64, 0, len(parts))

	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)

		if err != nil {
			return nil, fmt.Errorf("imageIds must be a comma-separated list of integers")
		}

		ids = append(ids, id)
	}

	return ids, nil
}
