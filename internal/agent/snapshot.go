package agent

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const shortContainerIDLength = 12

func (a *Agent) snapshot(ctx context.Context) agentapi.Report {
	report := agentapi.Report{ProtocolVersion: agentapi.ProtocolVersion, AgentVersion: a.version}

	if err := a.collect(ctx, &report); err != nil {
		slog.Warn("reading Docker failed", "error", err)

		report = agentapi.Report{
			ProtocolVersion: agentapi.ProtocolVersion,
			AgentVersion:    a.version,
			DockerError:     limit(err.Error(), agentapi.MaxTextLength),
		}
	}

	report.Fingerprint = fingerprint(report)

	return report
}

func (a *Agent) collect(ctx context.Context, report *agentapi.Report) error {
	epoch := a.cache.currentEpoch()

	info, err := a.info(ctx, epoch)

	if err != nil {
		return err
	}

	report.Host = agentapi.HostInfo{
		Hostname:      info.Name,
		DockerVersion: info.ServerVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		CPUs:          info.NCPU,
		MemoryBytes:   info.MemTotal,
	}

	containers, err := a.docker.containers(ctx)

	if err != nil {
		return err
	}

	digests, err := a.imageDigests(ctx, epoch)

	if err != nil {
		return err
	}

	used := make(map[string]bool)

	for _, c := range containers {
		details, err := a.containerDetails(ctx, epoch, c.ID)

		if err != nil {
			return err
		}

		if strings.HasPrefix(c.Image, "sha256:") && details.image != "" {
			c.Image = details.image
		}

		if a.cfg.ProxyOnly && !agentapi.ParseImageRef(c.Image).OnHost(a.proxyHost) {
			continue
		}

		refImageID, err := a.refImageID(ctx, epoch, c.Image)

		if err != nil {
			return err
		}

		report.Containers = append(report.Containers, agentapi.Container{
			ID:             c.ID,
			Name:           containerName(c),
			Image:          c.Image,
			ImageID:        c.ImageID,
			RefImageID:     refImageID,
			State:          c.State,
			Health:         healthFromStatus(c.Status),
			ExitCode:       details.exitCode,
			ComposeProject: c.Labels["com.docker.compose.project"],
			ComposeService: c.Labels["com.docker.compose.service"],
			Created:        c.Created,
			StartedAt:      details.startedAt,
			FinishedAt:     details.finishedAt,
			Ports:          publishedPorts(c.Ports),
		})

		used[c.ImageID] = true
		used[refImageID] = true
	}

	if len(report.Containers) > agentapi.MaxContainers {
		return fmt.Errorf("this host has %d containers; dockvmap accepts at most %d per host", len(report.Containers), agentapi.MaxContainers)
	}

	for id, repoDigests := range digests {
		if used[id] {
			report.Images = append(report.Images, agentapi.Image{ID: id, RepoDigests: repoDigests})
		}
	}

	slices.SortFunc(report.Containers, func(x, y agentapi.Container) int { return cmp.Compare(x.ID, y.ID) })
	slices.SortFunc(report.Images, func(x, y agentapi.Image) int { return cmp.Compare(x.ID, y.ID) })

	return nil
}

func (a *Agent) info(ctx context.Context, epoch int) (dockerInfo, error) {
	var cached *dockerInfo

	a.cache.read(func() { cached = a.cache.info })

	if cached != nil {
		return *cached, nil
	}

	info, err := a.docker.info(ctx)

	if err != nil {
		return dockerInfo{}, err
	}

	a.cache.write(epoch, func() { a.cache.info = &info })

	return info, nil
}

func (a *Agent) imageDigests(ctx context.Context, epoch int) (map[string][]string, error) {
	var cached map[string][]string

	a.cache.read(func() { cached = a.cache.images })

	if cached != nil {
		return cached, nil
	}

	images, err := a.docker.images(ctx)

	if err != nil {
		return nil, err
	}

	digests := make(map[string][]string, len(images))

	for _, image := range images {
		digests[image.ID] = image.RepoDigests
	}

	a.cache.write(epoch, func() { a.cache.images = digests })

	return digests, nil
}

func (a *Agent) containerDetails(ctx context.Context, epoch int, id string) (containerDetails, error) {
	var cached containerDetails
	var found bool

	a.cache.read(func() { cached, found = a.cache.containers[id] })

	if found {
		return cached, nil
	}

	details, exists, err := a.docker.containerDetails(ctx, id)

	if err != nil || !exists {
		return details, err
	}

	a.cache.write(epoch, func() { a.cache.containers[id] = details })

	return details, nil
}

func (a *Agent) refImageID(ctx context.Context, epoch int, ref string) (string, error) {
	if strings.HasPrefix(ref, "sha256:") {
		return "", nil
	}

	var cached string
	var found bool

	a.cache.read(func() { cached, found = a.cache.refImages[ref] })

	if found {
		return cached, nil
	}

	id, err := a.docker.imageID(ctx, ref)

	if err != nil {
		return "", err
	}

	a.cache.write(epoch, func() { a.cache.refImages[ref] = id })

	return id, nil
}

func publishedPorts(ports []dockerPort) []agentapi.Port {
	var published []agentapi.Port

	for _, p := range ports {
		if p.PublicPort > 0 {
			published = append(published, agentapi.Port{IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
		}
	}

	slices.SortFunc(published, func(x, y agentapi.Port) int {
		return cmp.Or(cmp.Compare(x.PublicPort, y.PublicPort), cmp.Compare(x.Type, y.Type), cmp.Compare(x.IP, y.IP))
	})

	return published
}

func fingerprint(report agentapi.Report) string {
	report.Fingerprint = ""
	encoded, _ := json.Marshal(report)
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}

func healthFromStatus(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	}

	return ""
}

func containerName(c dockerContainer) string {
	for _, name := range c.Names {
		if trimmed := strings.TrimPrefix(name, "/"); !strings.Contains(trimmed, "/") {
			return trimmed
		}
	}

	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}

	return c.ID[:min(shortContainerIDLength, len(c.ID))]
}

func limit(value string, max int) string {
	if len(value) <= max {
		return value
	}

	return strings.ToValidUTF8(value[:max], "")
}
