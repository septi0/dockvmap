package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/store"
)

var (
	ErrInvalidHost              = errors.New("invalid host")
	ErrHostAlreadyExists        = errors.New("host already exists")
	ErrHostNotFound             = errors.New("host not found")
	ErrInvalidAgentReport       = errors.New("invalid agent report")
	ErrUnsupportedAgentProtocol = errors.New("unsupported agent protocol")
	ErrFullReportNeeded         = errors.New("full report needed")
)

const HostOfflineAfter = 3 * time.Minute

const crashBadgeWindow = 7 * 24 * time.Hour

const (
	maxHostNameLength       = 100
	maxAgentVersionLength   = 64
	maxFingerprintLength    = 128
	maxImageRefLength       = 512
	maxDockerIDLength       = 128
	maxContainerStateLength = 32
	maxRepoDigestsPerItem   = 100
	maxPortsPerContainer    = 100
	maxIPLength             = 64
)

var containerHealthStates = []string{"", "healthy", "unhealthy", "starting"}

var portTypes = []string{"tcp", "udp", "sctp"}

type hostStore interface {
	CreateHost(ctx context.Context, name, tokenHash string) (int64, error)
	ListHosts(ctx context.Context) ([]model.Host, error)
	GetHostByID(ctx context.Context, id int64) (*model.Host, error)
	HostIDByTokenHash(ctx context.Context, tokenHash string) (int64, bool, error)
	UpdateHostToken(ctx context.Context, id int64, tokenHash string) (bool, error)
	DeleteHost(ctx context.Context, id int64) (bool, error)
	TouchHost(ctx context.Context, id int64, agentVersion string, protocolVersion int, seenAt *time.Time) error
	ReplaceHostSnapshot(ctx context.Context, id int64, snapshot model.HostSnapshot, seenAt time.Time) error
	RecordHostDockerError(ctx context.Context, id int64, snapshot model.HostSnapshot, seenAt time.Time) error
	ListHostContainers(ctx context.Context, hostID int64) ([]model.HostContainer, error)
	ListContainersByRepository(ctx context.Context, repository string) ([]model.HostContainer, error)
	ListAllHostContainers(ctx context.Context) ([]model.HostContainer, error)
	GetImage(ctx context.Context, name string) (*model.Image, error)
	GetImageByID(ctx context.Context, imageId int64) (*model.Image, error)
}

type ProxyTarget struct {
	Host       string
	VirtualTag string
}

type auditHostData struct {
	Name string `json:"name"`
}

type Hosts struct {
	store hostStore
	audit auditRecorder
	now   func() time.Time
}

func NewHosts(store hostStore, audit auditRecorder) *Hosts {
	return &Hosts{store: store, audit: audit, now: time.Now}
}

func (h *Hosts) Create(ctx context.Context, name string) (int64, string, error) {
	name = strings.TrimSpace(name)

	if name == "" {
		return 0, "", fmt.Errorf("%w: name is required", ErrInvalidHost)
	}

	if len(name) > maxHostNameLength {
		return 0, "", fmt.Errorf("%w: name must be at most %d characters", ErrInvalidHost, maxHostNameLength)
	}

	token, err := generateSecretToken()

	if err != nil {
		return 0, "", err
	}

	id, err := h.store.CreateHost(ctx, name, hashSecretToken(token))

	if errors.Is(err, store.ErrHostNameConflict) {
		return 0, "", ErrHostAlreadyExists
	}

	if err != nil {
		return 0, "", err
	}

	recordAudit(ctx, h.audit, AuditTypeHostCreated, auditHostData{Name: name})

	return id, token, nil
}

func (h *Hosts) List(ctx context.Context, proxy ProxyTarget) ([]model.Host, error) {
	hosts, containers, err := h.trackedContainers(ctx, proxy)

	if err != nil {
		return nil, err
	}

	indexes := make(map[int64]int, len(hosts))

	for i := range hosts {
		indexes[hosts[i].ID] = i
	}

	for _, c := range containers {
		index, known := indexes[c.HostID]

		if !known || !hosts[index].Current() {
			continue
		}

		host := &hosts[index]
		host.Rollout.Add(c.Tracking)

		if c.Tracking != nil {
			host.ProxyContainers++
		}

		if h.crashedRecently(c) {
			host.CrashedContainers++
		}

		if c.Health != nil && *c.Health == "unhealthy" {
			host.UnhealthyContainers++
		}

		if c.Running() && c.StartedAt != nil && (host.LastStartedAt == nil || c.StartedAt.After(*host.LastStartedAt)) {
			host.LastStartedAt = c.StartedAt
		}
	}

	return hosts, nil
}

func (h *Hosts) crashedRecently(c model.HostContainer) bool {
	if !c.Crashed() {
		return false
	}

	return c.Restarting() || c.FinishedAt == nil || h.now().Sub(*c.FinishedAt) < crashBadgeWindow
}

func (h *Hosts) ImageRollouts(ctx context.Context, proxy ProxyTarget) ([]model.ImageRollout, error) {
	hosts, containers, err := h.trackedContainers(ctx, proxy)

	if err != nil {
		return nil, err
	}

	byHost := make(map[int64]model.Host, len(hosts))

	for _, host := range hosts {
		byHost[host.ID] = host
	}

	type imageHost struct{ image, host int64 }

	byImage := make(map[int64]*model.ImageRollout)
	seenStale := make(map[imageHost]bool)

	for _, c := range containers {
		if c.Tracking == nil || !c.Tracking.InRollout {
			continue
		}

		id := *c.Tracking.ImageID
		rollout, ok := byImage[id]

		if !ok {
			rollout = &model.ImageRollout{ImageID: id}
			byImage[id] = rollout
		}

		host := byHost[c.HostID]

		if host.Current() {
			rollout.Rollout.Add(c.Tracking)
		} else if key := (imageHost{id, c.HostID}); !seenStale[key] {
			seenStale[key] = true

			if host.Online && host.DockerError != nil {
				rollout.HostsDockerError++
			} else {
				rollout.HostsOffline++
			}
		}
	}

	result := make([]model.ImageRollout, 0, len(byImage))

	for _, rollout := range byImage {
		result = append(result, *rollout)
	}

	slices.SortFunc(result, func(a, b model.ImageRollout) int { return cmp.Compare(a.ImageID, b.ImageID) })

	return result, nil
}

func (h *Hosts) trackedContainers(ctx context.Context, proxy ProxyTarget) ([]model.Host, []model.HostContainer, error) {
	hosts, err := h.listHosts(ctx)

	if err != nil {
		return nil, nil, err
	}

	containers, err := h.store.ListAllHostContainers(ctx)

	if err != nil {
		return nil, nil, err
	}

	if err := h.track(ctx, containers, proxy); err != nil {
		return nil, nil, err
	}

	return hosts, containers, nil
}

func (h *Hosts) listHosts(ctx context.Context) ([]model.Host, error) {
	hosts, err := h.store.ListHosts(ctx)

	if err != nil {
		return nil, err
	}

	for i := range hosts {
		h.markStatus(&hosts[i])
	}

	return hosts, nil
}

func (h *Hosts) Get(ctx context.Context, id int64, proxy ProxyTarget) (*model.HostDetails, error) {
	host, err := h.getHost(ctx, id)

	if err != nil {
		return nil, err
	}

	containers, err := h.store.ListHostContainers(ctx, id)

	if err != nil {
		return nil, err
	}

	if err := h.track(ctx, containers, proxy); err != nil {
		return nil, err
	}

	for i := range containers {
		containers[i].RecentlyCrashed = h.crashedRecently(containers[i])
	}

	return &model.HostDetails{Host: *host, Containers: containers}, nil
}

func (h *Hosts) RegenerateToken(ctx context.Context, id int64) (string, error) {
	host, err := h.getHost(ctx, id)

	if err != nil {
		return "", err
	}

	token, err := generateSecretToken()

	if err != nil {
		return "", err
	}

	updated, err := h.store.UpdateHostToken(ctx, id, hashSecretToken(token))

	if err != nil {
		return "", err
	}

	if !updated {
		return "", fmt.Errorf("%w: %d", ErrHostNotFound, id)
	}

	recordAudit(ctx, h.audit, AuditTypeHostTokenRegenerated, auditHostData{Name: host.Name})

	return token, nil
}

func (h *Hosts) Delete(ctx context.Context, id int64) error {
	host, err := h.getHost(ctx, id)

	if err != nil {
		return err
	}

	deleted, err := h.store.DeleteHost(ctx, id)

	if err != nil {
		return err
	}

	if !deleted {
		return fmt.Errorf("%w: %d", ErrHostNotFound, id)
	}

	recordAudit(ctx, h.audit, AuditTypeHostDeleted, auditHostData{Name: host.Name})

	return nil
}

func (h *Hosts) Authenticate(ctx context.Context, token string) (int64, bool, error) {
	if token == "" {
		return 0, false, nil
	}

	return h.store.HostIDByTokenHash(ctx, hashSecretToken(token))
}

func (h *Hosts) Report(ctx context.Context, hostID int64, report agentapi.Report) error {
	if err := h.checkAgent(ctx, hostID, report.ProtocolVersion, report.AgentVersion, report.Fingerprint); err != nil {
		return err
	}

	snapshot, err := snapshotFromReport(report)

	if err != nil {
		return err
	}

	host, err := h.getHost(ctx, hostID)

	if err != nil {
		return err
	}

	now := h.now().UTC()

	if host.Fingerprint != nil && *host.Fingerprint == snapshot.Fingerprint {
		return h.store.TouchHost(ctx, hostID, snapshot.AgentVersion, snapshot.ProtocolVersion, &now)
	}

	if snapshot.DockerError != nil {
		return h.store.RecordHostDockerError(ctx, hostID, snapshot, now)
	}

	return h.store.ReplaceHostSnapshot(ctx, hostID, snapshot, now)
}

func (h *Hosts) Heartbeat(ctx context.Context, hostID int64, heartbeat agentapi.Heartbeat) error {
	if err := h.checkAgent(ctx, hostID, heartbeat.ProtocolVersion, heartbeat.AgentVersion, heartbeat.Fingerprint); err != nil {
		return err
	}

	host, err := h.getHost(ctx, hostID)

	if err != nil {
		return err
	}

	if host.Fingerprint == nil || *host.Fingerprint != heartbeat.Fingerprint {
		return ErrFullReportNeeded
	}

	now := h.now().UTC()

	return h.store.TouchHost(ctx, hostID, heartbeat.AgentVersion, heartbeat.ProtocolVersion, &now)
}

func (h *Hosts) checkAgent(ctx context.Context, hostID int64, protocolVersion int, agentVersion, fingerprint string) error {
	if len(agentVersion) > maxAgentVersionLength {
		return fmt.Errorf("%w: agent version longer than %d characters", ErrInvalidAgentReport, maxAgentVersionLength)
	}

	if protocolVersion != agentapi.ProtocolVersion {
		if err := h.store.TouchHost(ctx, hostID, agentVersion, protocolVersion, nil); err != nil {
			return err
		}

		return fmt.Errorf("%w: agent speaks protocol %d, this dockvmap speaks %d; run the agent version that matches dockvmap", ErrUnsupportedAgentProtocol, protocolVersion, agentapi.ProtocolVersion)
	}

	if fingerprint == "" || len(fingerprint) > maxFingerprintLength {
		return fmt.Errorf("%w: fingerprint is missing or too long", ErrInvalidAgentReport)
	}

	return nil
}

func (h *Hosts) ImageContainers(ctx context.Context, imageID int64, proxy ProxyTarget) (*model.ImageContainers, error) {
	image, err := h.store.GetImageByID(ctx, imageID)

	if err != nil {
		return nil, err
	}

	if image == nil {
		return nil, fmt.Errorf("%w: %d", ErrImageNotFound, imageID)
	}

	containers, err := h.store.ListContainersByRepository(ctx, image.Name)

	if err != nil {
		return nil, err
	}

	byHost := make(map[int64][]model.HostContainer)

	for _, c := range containers {
		if !onProxy(c, proxy) {
			continue
		}

		c.Tracking = trackContainer(c, image, proxy.VirtualTag)
		byHost[c.HostID] = append(byHost[c.HostID], c)
	}

	hosts, err := h.listHosts(ctx)

	if err != nil {
		return nil, err
	}

	result := &model.ImageContainers{Hosts: make([]model.HostDetails, 0), ConfiguredHosts: len(hosts)}

	for _, host := range hosts {
		hostContainers, ok := byHost[host.ID]

		if !ok {
			continue
		}

		if host.Current() {
			for _, c := range hostContainers {
				result.Rollout.Add(c.Tracking)
			}
		}

		result.Hosts = append(result.Hosts, model.HostDetails{Host: host, Containers: hostContainers})
	}

	return result, nil
}

func (h *Hosts) getHost(ctx context.Context, id int64) (*model.Host, error) {
	if id < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidHost)
	}

	host, err := h.store.GetHostByID(ctx, id)

	if err != nil {
		return nil, err
	}

	if host == nil {
		return nil, fmt.Errorf("%w: %d", ErrHostNotFound, id)
	}

	h.markStatus(host)

	return host, nil
}

func (h *Hosts) markStatus(host *model.Host) {
	host.AgentUpdateNeeded = host.ProtocolVersion != nil && *host.ProtocolVersion != agentapi.ProtocolVersion
	host.Online = host.LastSeenAt != nil && !host.AgentUpdateNeeded && h.now().Sub(*host.LastSeenAt) < HostOfflineAfter
}

func (h *Hosts) track(ctx context.Context, containers []model.HostContainer, proxy ProxyTarget) error {
	images := make(map[string]*model.Image)

	for i := range containers {
		c := &containers[i]

		if !onProxy(*c, proxy) {
			continue
		}

		image, seen := images[c.ImageRepository]

		if !seen {
			var err error

			if image, err = h.store.GetImage(ctx, c.ImageRepository); err != nil {
				return err
			}

			images[c.ImageRepository] = image
		}

		c.Tracking = trackContainer(*c, image, proxy.VirtualTag)
	}

	return nil
}

func onProxy(c model.HostContainer, proxy ProxyTarget) bool {
	return agentapi.ImageRef{Host: c.ImageHost}.OnHost(proxy.Host)
}

func trackContainer(c model.HostContainer, image *model.Image, virtualTag string) *model.ContainerTracking {
	tracking := &model.ContainerTracking{ImageName: c.ImageRepository}
	followsVirtualTag := c.ImageTag != nil && *c.ImageTag == virtualTag

	if c.ImageTag != nil && !followsVirtualTag {
		tracking.PinnedTag = c.ImageTag
	}

	if image == nil {
		tracking.Status = model.ContainerUnknownImage

		return tracking
	}

	tracking.ImageID = &image.ID
	digest, _ := image.TagDigest()

	switch {
	case digest == nil || len(c.RunningDigests) == 0:
		tracking.Status = model.ContainerVersionUnknown
	case slices.Contains(c.RunningDigests, *digest):
		tracking.Status = model.ContainerUpToDate
	case slices.Contains(c.RefDigests, *digest):
		tracking.Status = model.ContainerPulledNotRecreated
	default:
		tracking.Status = model.ContainerOutdated
	}

	tracking.InRollout = (c.Running() || c.Restarting()) && followsVirtualTag && tracking.Status != model.ContainerVersionUnknown

	return tracking
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
	case len(c.ComposeProject) > agentapi.MaxTextLength || len(c.ComposeService) > agentapi.MaxTextLength:
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
		ContainerID:     c.ID,
		Name:            c.Name,
		Image:           c.Image,
		ImageHost:       ref.Host,
		ImageRepository: ref.Repository,
		ImageTag:        optional(ref.Tag),
		ImageDigest:     optional(ref.Digest),
		RunningDigests:  knownDigests(digests, c.ImageID),
		State:           c.State,
		Health:          optional(c.Health),
		ExitCode:        c.ExitCode,
		ComposeProject:  optional(c.ComposeProject),
		ComposeService:  optional(c.ComposeService),
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
