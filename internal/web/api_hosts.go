package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/septi0/dockvmap/internal/service"
)

func (w *Web) apiListHosts(rw http.ResponseWriter, r *http.Request) {
	hosts, err := w.hosts.List(r.Context(), w.proxyTarget(r))

	if err != nil {
		apiError(rw, http.StatusInternalServerError, "internal server error")
		return
	}

	responses := make([]hostResponse, 0, len(hosts))

	for _, host := range hosts {
		responses = append(responses, newHostResponse(host))
	}

	apiJSON(rw, http.StatusOK, responses)
}

func (w *Web) apiCreateHost(rw http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[hostRequest](rw, r)
	if !ok {
		return
	}

	id, err := w.hosts.Create(r.Context(), request.Name, request.Address)

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusCreated, map[string]int64{"id": id})
}

func (w *Web) apiUpdateHost(rw http.ResponseWriter, r *http.Request) {
	id, ok := hostIDParam(rw, r)
	if !ok {
		return
	}

	request, ok := decodeJSON[hostRequest](rw, r)
	if !ok {
		return
	}

	if err := w.hosts.Update(r.Context(), id, request.Name, request.Address); err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "updated"})
}

func (w *Web) apiGetHost(rw http.ResponseWriter, r *http.Request) {
	id, ok := hostIDParam(rw, r)
	if !ok {
		return
	}

	details, err := w.hosts.Get(r.Context(), id, w.proxyTarget(r))

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusOK, hostDetailsResponse{
		Host:       newHostResponse(details.Host),
		Containers: newHostContainerResponses(details.Containers),
	})
}

func (w *Web) apiDeleteHost(rw http.ResponseWriter, r *http.Request) {
	id, ok := hostIDParam(rw, r)
	if !ok {
		return
	}

	if err := w.hosts.Delete(r.Context(), id); err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "deleted"})
}

func (w *Web) apiTrustHostAgent(rw http.ResponseWriter, r *http.Request) {
	id, ok := hostIDParam(rw, r)
	if !ok {
		return
	}

	if err := w.hosts.TrustNewAgent(r.Context(), id); err != nil {
		apiServiceError(rw, err)
		return
	}

	apiJSON(rw, http.StatusOK, map[string]string{"status": "trusted"})
}

func (w *Web) apiGetImageHosts(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "image id must be a valid integer")
		return
	}

	result, err := w.hosts.ImageContainers(r.Context(), id, w.proxyTarget(r))

	if errors.Is(err, service.ErrImageNotFound) {
		apiError(rw, http.StatusNotFound, "image not found")
		return
	}

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	hosts := make([]hostDetailsResponse, 0, len(result.Hosts))

	for _, entry := range result.Hosts {
		hosts = append(hosts, hostDetailsResponse{
			Host:       newHostResponse(entry.Host),
			Containers: newHostContainerResponses(entry.Containers),
		})
	}

	apiJSON(rw, http.StatusOK, imageContainersResponse{
		Hosts:           hosts,
		Rollout:         newRolloutResponse(result.Rollout),
		ConfiguredHosts: result.ConfiguredHosts,
	})
}

func (w *Web) apiListImageRollouts(rw http.ResponseWriter, r *http.Request) {
	rollouts, err := w.hosts.ImageRollouts(r.Context(), w.proxyTarget(r))

	if err != nil {
		apiServiceError(rw, err)
		return
	}

	responses := make([]imageRolloutResponse, 0, len(rollouts))

	for _, rollout := range rollouts {
		responses = append(responses, imageRolloutResponse{
			ImageID:          rollout.ImageID,
			rolloutResponse:  newRolloutResponse(rollout.Rollout),
			HostsOffline:     rollout.HostsOffline,
			HostsDockerError: rollout.HostsDockerError,
		})
	}

	apiJSON(rw, http.StatusOK, responses)
}

func hostIDParam(rw http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)

	if err != nil {
		apiError(rw, http.StatusBadRequest, "host id must be a valid integer")
		return 0, false
	}

	return id, true
}
