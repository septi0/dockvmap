package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

const maxImagesLimit = 200

const (
	refreshInlineWaitBudget   = 2 * time.Second
	refreshInlineWaitInterval = 150 * time.Millisecond
)

type listImagesResponse struct {
	Images []imageResponse `json:"images"`
	Total  int64           `json:"total"`
}

func parseImageListFilters(r *http.Request) (model.ImageListFilters, error) {
	pagination, err := parsePagination(r, maxImagesLimit)

	if err != nil {
		return model.ImageListFilters{}, err
	}

	updateAvailable, err := parseBoolParam(r, "updateAvailable")

	if err != nil {
		return model.ImageListFilters{}, err
	}

	return model.ImageListFilters{
		Pagination:      pagination,
		Search:          strings.TrimSpace(r.URL.Query().Get("search")),
		UpdateAvailable: updateAvailable,
	}, nil
}

func (w *Web) apiListImages(rw http.ResponseWriter, r *http.Request) {
	filters, err := parseImageListFilters(r)

	if err != nil {
		apiError(rw, http.StatusBadRequest, err.Error())
		return
	}

	images, err := w.images.List(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")

		return
	}

	total, err := w.images.Count(r.Context(), filters)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")

		return
	}

	responses := make([]imageResponse, 0, len(images))

	for _, image := range images {
		responses = append(responses, newImageResponse(image))
	}

	apiJSON(rw, http.StatusOK, listImagesResponse{
		Images: responses,
		Total:  total,
	})
}

func (w *Web) apiCreateImage(rw http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[createImageRequest](rw, r)
	if !ok {
		return
	}

	availableTags, _ := w.discoveries.CachedTags(r.Context(), req.RegistryID, req.Repository)

	err := w.images.Create(r.Context(), model.Image{
		Name:       req.Name,
		RegistryID: req.RegistryID,
		Repository: req.Repository,
		Tag:        req.Tag,
	}, availableTags)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageAlreadyExists):
			apiError(rw, http.StatusConflict, "image name already exists")

		case errors.Is(err, service.ErrUpstreamNotFound), errors.Is(err, service.ErrUpstreamUnauthorized):
			apiError(rw, http.StatusNotFound, "repository does not exist or may require authentication")

		case errors.Is(err, service.ErrTagUnavailable):
			apiError(rw, http.StatusConflict, "upstream image tag is not available")

		case errors.Is(err, service.ErrUpstreamUnavailable):
			apiError(rw, http.StatusBadGateway, "upstream registry check failed")

		case errors.Is(err, service.ErrFailedToRefreshTags):
			apiJSON(rw, http.StatusCreated, map[string]any{
				"status":            "created",
				"refreshSuccessful": false,
			})

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusCreated, map[string]any{
		"status":            "created",
		"refreshSuccessful": true,
	})
}

func (w *Web) apiInspectRepository(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[inspectRepositoryRequest](rw, r)
	if !ok {
		return
	}

	discovery, err := w.discoveries.Check(r.Context(), request.Registry, request.Repository)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrUpstreamNotFound),
			errors.Is(err, service.ErrUpstreamUnauthorized):

			apiError(rw, http.StatusNotFound, "repository does not exist or may require authentication")

		case errors.Is(err, service.ErrUpstreamUnavailable):
			apiError(rw, http.StatusBadGateway, "upstream registry check failed")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, newDiscoveryResponse(discovery))
}

func (w *Web) apiGetDiscovery(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "discovery id must be a valid integer")

		return
	}

	discovery, err := w.discoveries.Get(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrTagDiscoveryNotFound):
			apiError(rw, http.StatusNotFound, "discovery not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, newDiscoveryResponse(*discovery))
}

func (w *Web) apiDeleteImage(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	deleted, err := w.images.Delete(r.Context(), id)

	if err != nil {
		if errors.Is(err, service.ErrInvalidImage) {
			apiError(rw, http.StatusBadRequest, err.Error())

			return
		}

		apiError(rw, http.StatusInternalServerError, "internal server error")

		return
	}

	if !deleted {
		apiError(rw, http.StatusNotFound, "image not found")

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func (w *Web) apiRefreshImageTags(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	started, err := w.images.StartBackgroundRefresh(id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	if !started {
		apiJSON(rw, http.StatusAccepted, map[string]any{"status": "running"})

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), refreshInlineWaitBudget)
	defer cancel()

	ticker := time.NewTicker(refreshInlineWaitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			apiJSON(rw, http.StatusAccepted, map[string]any{"status": "running"})

			return

		case <-ticker.C:
			img, err := w.images.GetByID(context.WithoutCancel(r.Context()), id)

			if err != nil || img == nil || img.RefreshStatus == model.RefreshStatusRunning {
				continue
			}

			if img.LastCheckError != nil {
				apiJSON(rw, http.StatusOK, map[string]any{
					"status": "error",
					"error":  *img.LastCheckError,
				})

				return
			}

			apiJSON(rw, http.StatusOK, map[string]any{"status": "refreshed"})

			return
		}
	}
}

func (w *Web) apiUpdateImageTag(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	request, ok := decodeJSON[updateImageTagRequest](rw, r)
	if !ok {
		return
	}

	source := model.TagHistorySourceManual

	if request.Source == string(model.TagHistorySourceRestore) {
		source = model.TagHistorySourceRestore
	}

	err = w.images.UpdateTag(r.Context(), id, request.Tag, source)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		case errors.Is(err, service.ErrTagUnavailable):
			apiError(rw, http.StatusConflict, "image tag is not available")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) apiRenameImage(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	request, ok := decodeJSON[renameImageRequest](rw, r)
	if !ok {
		return
	}

	err = w.images.Rename(r.Context(), id, request.Name)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		case errors.Is(err, service.ErrImageAlreadyExists):
			apiError(rw, http.StatusConflict, "image name already exists")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "renamed"})
}

func (w *Web) apiGetImage(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	image, err := w.images.GetByID(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, newImageResponse(*image))
}

func (w *Web) apiGetImageTags(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	image, err := w.images.GetByID(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	tags, err := w.images.GetTags(r.Context(), id)

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")

		return
	}

	response := newImageTagsResponse(tags, image.Tag)

	apiJSON(rw, http.StatusOK, response)
}

func (w *Web) apiGetImageTagHistory(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	history, err := w.images.GetTagHistory(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, newTagHistoryResponse(history))
}

func (w *Web) apiMarkImageTagsAsSeen(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")

		return
	}

	count, err := w.images.MarkTagsAsSeen(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidImage):
			apiError(rw, http.StatusBadRequest, err.Error())

		case errors.Is(err, service.ErrImageNotFound):
			apiError(rw, http.StatusNotFound, "image not found")

		default:
			apiError(rw, http.StatusInternalServerError, "internal server error")
		}

		return
	}

	apiJSON(rw, http.StatusOK, map[string]any{
		"status":           "marked",
		"tagsMarkedAsSeen": count,
	})
}
