package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/agentclient"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
)

const (
	hostOfflineAfterPolls = 3
	agentPollTimeout      = 10 * time.Second
	agentPollWorkers      = 8
	livePollTimeout       = 3 * time.Second
	livePollMaxAge        = 5 * time.Second
)

var containerHealthStates = []string{"", "healthy", "unhealthy", "starting"}

var portTypes = []string{"tcp", "udp", "sctp"}

type agentDialer interface {
	Adopt(ctx context.Context, address, pinned string) (agentapi.Adoption, string, error)
	Snapshot(ctx context.Context, address, pinned, fingerprint, proxyHost string) (*agentapi.Report, error)
}

func (h *Hosts) PollAll(ctx context.Context) (int, error) {
	hosts, err := h.store.ListHosts(ctx)

	if err != nil {
		return 0, err
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errs   []error
		polled int
	)

	workers := make(chan struct{}, agentPollWorkers)

	for _, host := range hosts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			workers <- struct{}{}
			defer func() { <-workers }()

			err := h.poll(ctx, host.ID, 0, agentPollTimeout)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errs = append(errs, err)
			} else {
				polled++
			}
		}()
	}

	wg.Wait()

	return polled, errors.Join(errs...)
}

func (h *Hosts) poll(ctx context.Context, id int64, maxAge, timeout time.Duration) error {
	lock := h.pollLock(id)

	if maxAge > 0 {
		if !lock.TryLock() {
			return nil
		}
	} else {
		lock.Lock()
	}

	defer lock.Unlock()

	h.mu.Lock()
	last := h.polled[id]
	h.mu.Unlock()

	if maxAge > 0 && h.now().Sub(last) < maxAge {
		return nil
	}

	host, err := h.store.GetHostByID(ctx, id)

	if err != nil || host == nil {
		return err
	}

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	report, adopted, err := h.fetch(pollCtx, host)

	if ctx.Err() != nil {
		return ctx.Err()
	}

	h.mu.Lock()
	h.polled[id] = h.now()
	h.mu.Unlock()

	if adopted != "" {
		if err := h.store.SetHostAgent(ctx, host.ID, &adopted); err != nil {
			return err
		}

		slog.Info("adopted host agent", "host", host.Name, "address", host.Address, "fingerprint", adopted)
	}

	if err == nil {
		err = h.apply(ctx, host, report)

		if err != nil && !errors.Is(err, ErrInvalidAgentReport) && !errors.Is(err, ErrUnsupportedAgentProtocol) {
			return err
		}
	}

	if err != nil {
		return h.pollFailed(ctx, host, err)
	}

	if host.PollError != nil {
		slog.Info("host agent answers again", "host", host.Name)
	}

	return nil
}

func (h *Hosts) pollLock(id int64) *sync.Mutex {
	h.mu.Lock()
	defer h.mu.Unlock()

	lock, ok := h.polls[id]

	if !ok {
		lock = &sync.Mutex{}
		h.polls[id] = lock
	}

	return lock
}

func (h *Hosts) fetch(ctx context.Context, host *model.Host) (*agentapi.Report, string, error) {
	pinned, fingerprint, adopted := "", "", ""

	if host.AgentFingerprint != nil {
		pinned = *host.AgentFingerprint
	}

	if host.ReportFingerprint != nil {
		fingerprint = *host.ReportFingerprint
	}

	if pinned == "" {
		_, seen, err := h.agents.Adopt(ctx, host.Address, "")

		if err != nil {
			return nil, "", err
		}

		pinned, fingerprint, adopted = seen, "", seen
	}

	report, err := h.agents.Snapshot(ctx, host.Address, pinned, fingerprint, h.proxyHost)

	if errors.Is(err, agentclient.ErrNotAdopted) {
		if _, _, err = h.agents.Adopt(ctx, host.Address, pinned); err == nil {
			report, err = h.agents.Snapshot(ctx, host.Address, pinned, fingerprint, h.proxyHost)
		}
	}

	return report, adopted, err
}

func (h *Hosts) apply(ctx context.Context, host *model.Host, report *agentapi.Report) error {
	now := h.now().UTC()

	if report == nil {
		agentVersion, protocolVersion := "", 0

		if host.AgentVersion != nil {
			agentVersion = *host.AgentVersion
		}

		if host.ProtocolVersion != nil {
			protocolVersion = *host.ProtocolVersion
		}

		return h.store.TouchHost(ctx, host.ID, agentVersion, protocolVersion, &now)
	}

	if err := h.checkAgent(ctx, host.ID, report.ProtocolVersion, report.AgentVersion, report.Fingerprint); err != nil {
		return err
	}

	snapshot, err := snapshotFromReport(*report)

	if err != nil {
		return err
	}

	if snapshot.DockerError != nil {
		return h.store.RecordHostDockerError(ctx, host.ID, snapshot, now)
	}

	return h.store.ReplaceHostSnapshot(ctx, host.ID, snapshot, now)
}

func (h *Hosts) pollFailed(ctx context.Context, host *model.Host, err error) error {
	kind := model.HostPollFailed

	switch {
	case errors.Is(err, agentclient.ErrIdentityChanged):
		kind = model.HostPollIdentityChanged
	case errors.Is(err, agentclient.ErrPairedElsewhere):
		kind = model.HostPollPairedElsewhere
	}

	message := err.Error()

	if len(message) > agentapi.MaxTextLength {
		message = strings.ToValidUTF8(message[:agentapi.MaxTextLength], "")
	}

	if host.PollError == nil || *host.PollError != message {
		slog.Warn("polling host agent failed", "host", host.Name, "address", host.Address, "error", message)
	}

	return h.store.RecordHostPollError(ctx, host.ID, kind, message)
}

func (h *Hosts) checkAgent(ctx context.Context, hostID int64, protocolVersion int, agentVersion, fingerprint string) error {
	if len(agentVersion) > maxAgentVersionLength {
		return fmt.Errorf("%w: agent version longer than %d characters", ErrInvalidAgentReport, maxAgentVersionLength)
	}

	if protocolVersion != agentapi.ProtocolVersion {
		if err := h.store.TouchHost(ctx, hostID, agentVersion, protocolVersion, nil); err != nil {
			return err
		}

		return fmt.Errorf("%w: agent speaks protocol %d, this DockVMap speaks %d; run the agent version that matches DockVMap", ErrUnsupportedAgentProtocol, protocolVersion, agentapi.ProtocolVersion)
	}

	if fingerprint == "" || len(fingerprint) > maxFingerprintLength {
		return fmt.Errorf("%w: fingerprint is missing or too long", ErrInvalidAgentReport)
	}

	return nil
}

func snapshotFromReport(report agentapi.Report) (model.HostSnapshot, error) {
	if len(report.Containers) > agentapi.MaxContainers {
		return model.HostSnapshot{}, fmt.Errorf("%w: more than %d containers", ErrInvalidAgentReport, agentapi.MaxContainers)
	}

	if len(report.Images) > agentapi.MaxImages {
		return model.HostSnapshot{}, fmt.Errorf("%w: more than %d images", ErrInvalidAgentReport, agentapi.MaxImages)
	}

	if err := validateHostInfo(report.Host); err != nil {
		return model.HostSnapshot{}, err
	}

	if len(report.DockerError) > agentapi.MaxTextLength {
		return model.HostSnapshot{}, fmt.Errorf("%w: docker error longer than %d characters", ErrInvalidAgentReport, agentapi.MaxTextLength)
	}

	digests, err := imageDigests(report.Images)

	if err != nil {
		return model.HostSnapshot{}, err
	}

	snapshot := model.HostSnapshot{
		AgentVersion:    report.AgentVersion,
		ProtocolVersion: report.ProtocolVersion,
		Info:            hostInfo(report.Host),
		DockerError:     optional(report.DockerError),
		Containers:      make([]model.HostContainer, 0, len(report.Containers)),
		Fingerprint:     report.Fingerprint,
	}

	seen := make(map[string]bool, len(report.Containers))

	for _, c := range report.Containers {
		if err := validateContainer(c); err != nil {
			return model.HostSnapshot{}, err
		}

		if seen[c.ID] {
			return model.HostSnapshot{}, fmt.Errorf("%w: container %q is listed twice", ErrInvalidAgentReport, c.ID)
		}

		seen[c.ID] = true
		snapshot.Containers = append(snapshot.Containers, hostContainer(c, digests))
	}

	return snapshot, nil
}

func validateHostInfo(info agentapi.HostInfo) error {
	for _, field := range []string{info.Hostname, info.DockerVersion, info.OS, info.Architecture} {
		if len(field) > agentapi.MaxTextLength {
			return fmt.Errorf("%w: host info field longer than %d characters", ErrInvalidAgentReport, agentapi.MaxTextLength)
		}
	}

	if info.CPUs < 0 || info.MemoryBytes < 0 {
		return fmt.Errorf("%w: negative host resources", ErrInvalidAgentReport)
	}

	return nil
}

func validateContainer(c agentapi.Container) error {
	switch {
	case c.ID == "" || len(c.ID) > maxDockerIDLength:
		return fmt.Errorf("%w: container id is missing or too long", ErrInvalidAgentReport)
	case c.Name == "" || len(c.Name) > agentapi.MaxTextLength:
		return fmt.Errorf("%w: container %q name is missing or too long", ErrInvalidAgentReport, c.ID)
	case c.Image == "" || len(c.Image) > maxImageRefLength:
		return fmt.Errorf("%w: container %q image is missing or too long", ErrInvalidAgentReport, c.Name)
	case len(c.ImageID) > maxDockerIDLength || len(c.RefImageID) > maxDockerIDLength:
		return fmt.Errorf("%w: container %q image id is too long", ErrInvalidAgentReport, c.Name)
	case c.State == "" || len(c.State) > maxContainerStateLength:
		return fmt.Errorf("%w: container %q state is missing or too long", ErrInvalidAgentReport, c.Name)
	case !slices.Contains(containerHealthStates, c.Health):
		return fmt.Errorf("%w: container %q has unknown health %q", ErrInvalidAgentReport, c.Name, c.Health)
	case len(c.ComposeProject) > agentapi.MaxTextLength || len(c.ComposeService) > agentapi.MaxTextLength ||
		len(c.ComposeWorkingDir) > agentapi.MaxTextLength:
		return fmt.Errorf("%w: container %q field longer than %d characters", ErrInvalidAgentReport, c.Name, agentapi.MaxTextLength)
	case c.Created < 0 || c.StartedAt < 0 || c.FinishedAt < 0:
		return fmt.Errorf("%w: container %q has a negative time", ErrInvalidAgentReport, c.Name)
	case len(c.Ports) > maxPortsPerContainer:
		return fmt.Errorf("%w: container %q has more than %d ports", ErrInvalidAgentReport, c.Name, maxPortsPerContainer)
	}

	for _, p := range c.Ports {
		if !validPort(p.PrivatePort) || !validPort(p.PublicPort) || !slices.Contains(portTypes, p.Type) || len(p.IP) > maxIPLength {
			return fmt.Errorf("%w: container %q has an invalid port", ErrInvalidAgentReport, c.Name)
		}
	}

	return nil
}

func validPort(port int) bool {
	return port >= 0 && port <= 65535
}

func imageDigests(images []agentapi.Image) (map[string][]string, error) {
	digests := make(map[string][]string, len(images))

	for _, image := range images {
		if image.ID == "" || len(image.ID) > maxDockerIDLength {
			return nil, fmt.Errorf("%w: image id is missing or too long", ErrInvalidAgentReport)
		}

		if len(image.RepoDigests) > maxRepoDigestsPerItem {
			return nil, fmt.Errorf("%w: image %q has more than %d digests", ErrInvalidAgentReport, image.ID, maxRepoDigestsPerItem)
		}

		for _, repoDigest := range image.RepoDigests {
			_, digest, found := strings.Cut(repoDigest, "@")

			if found && oci.IsDigest(digest) && !slices.Contains(digests[image.ID], digest) {
				digests[image.ID] = append(digests[image.ID], digest)
			}
		}
	}

	return digests, nil
}

func hostContainer(c agentapi.Container, digests map[string][]string) model.HostContainer {
	ref := agentapi.ParseImageRef(c.Image)

	container := model.HostContainer{
		ContainerID:       c.ID,
		Name:              c.Name,
		Image:             c.Image,
		ImageHost:         ref.Host,
		ImageRepository:   ref.Repository,
		ImageTag:          optional(ref.Tag),
		ImageDigest:       optional(ref.Digest),
		RunningDigests:    knownDigests(digests, c.ImageID),
		State:             c.State,
		Health:            optional(c.Health),
		ExitCode:          c.ExitCode,
		ComposeProject:    optional(c.ComposeProject),
		ComposeService:    optional(c.ComposeService),
		ComposeWorkingDir: optional(c.ComposeWorkingDir),
	}

	if c.RefImageID != "" && c.RefImageID != c.ImageID {
		container.RefDigests = knownDigests(digests, c.RefImageID)
	}

	for _, p := range c.Ports {
		container.Ports = append(container.Ports, model.ContainerPort{IP: p.IP, PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
	}

	container.CreatedAt = unixTime(c.Created)
	container.StartedAt = unixTime(c.StartedAt)
	container.FinishedAt = unixTime(c.FinishedAt)

	return container
}

func knownDigests(digests map[string][]string, imageID string) []string {
	known := digests[imageID]

	if oci.IsDigest(imageID) && !slices.Contains(known, imageID) {
		known = append(slices.Clone(known), imageID)
	}

	return known
}

func hostInfo(info agentapi.HostInfo) model.HostInfo {
	result := model.HostInfo{
		Hostname:      optional(info.Hostname),
		DockerVersion: optional(info.DockerVersion),
		OS:            optional(info.OS),
		Architecture:  optional(info.Architecture),
	}

	if info.CPUs > 0 {
		result.CPUs = &info.CPUs
	}

	if info.MemoryBytes > 0 {
		result.MemoryBytes = &info.MemoryBytes
	}

	return result
}

func unixTime(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}

	t := time.Unix(seconds, 0).UTC()

	return &t
}

func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
