package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

var (
	ErrInvalidHost              = errors.New("invalid host")
	ErrHostAlreadyExists        = errors.New("host already exists")
	ErrHostAddressInUse         = errors.New("host address already in use")
	ErrHostNotFound             = errors.New("host not found")
	ErrInvalidAgentReport       = errors.New("invalid agent report")
	ErrUnsupportedAgentProtocol = errors.New("unsupported agent protocol")
)

const crashBadgeWindow = 7 * 24 * time.Hour

const (
	maxHostNameLength       = 100
	maxHostAddressLength    = 255
	maxAgentVersionLength   = 64
	maxFingerprintLength    = 128
	maxImageRefLength       = 512
	maxDockerIDLength       = 128
	maxContainerStateLength = 32
	maxRepoDigestsPerItem   = 100
	maxPortsPerContainer    = 100
	maxIPLength             = 64
)

type hostStore interface {
	CreateHost(ctx context.Context, name, address string) (int64, error)
	ListHosts(ctx context.Context) ([]model.Host, error)
	GetHostByID(ctx context.Context, id int64) (*model.Host, error)
	UpdateHost(ctx context.Context, id int64, name, address string) (bool, error)
	SetHostAgent(ctx context.Context, id int64, fingerprint *string) error
	RecordHostPollError(ctx context.Context, id int64, kind, message string) error
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
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

type auditHostUpdateData struct {
	Name       string `json:"name"`
	Address    string `json:"address"`
	OldName    string `json:"oldName"`
	OldAddress string `json:"oldAddress"`
}

type internalTagDigests interface {
	Address() string
	TagDigest(repository, tag string) string
}

type HostsDeps struct {
	Store            hostStore
	Audit            auditRecorder
	Agents           agentDialer
	PollInterval     time.Duration
	ProxyHost        string
	InternalRegistry internalTagDigests
}

type Hosts struct {
	store        hostStore
	audit        auditRecorder
	agents       agentDialer
	offlineAfter time.Duration
	proxyHost    string

	internalRegistry internalTagDigests
	now              func() time.Time

	mu     sync.Mutex
	polls  map[int64]*sync.Mutex
	polled map[int64]time.Time
}

func NewHosts(deps HostsDeps) *Hosts {
	return &Hosts{
		store:        deps.Store,
		audit:        deps.Audit,
		agents:       deps.Agents,
		offlineAfter: hostOfflineAfterPolls * deps.PollInterval,
		proxyHost:    deps.ProxyHost,

		internalRegistry: deps.InternalRegistry,
		now:              time.Now,
		polls:            make(map[int64]*sync.Mutex),
		polled:           make(map[int64]time.Time),
	}
}

func (h *Hosts) Create(ctx context.Context, name, address string) (int64, error) {
	name, address, err := validateHost(name, address)

	if err != nil {
		return 0, err
	}

	id, err := h.store.CreateHost(ctx, name, address)

	if err != nil {
		return 0, hostConflict(err)
	}

	recordAudit(ctx, h.audit, AuditTypeHostCreated, auditHostData{Name: name, Address: address})

	return id, nil
}

func (h *Hosts) Update(ctx context.Context, id int64, name, address string) error {
	host, err := h.getHost(ctx, id)

	if err != nil {
		return err
	}

	name, address, err = validateHost(name, address)

	if err != nil {
		return err
	}

	if name == host.Name && address == host.Address {
		return nil
	}

	updated, err := h.store.UpdateHost(ctx, id, name, address)

	if err != nil {
		return hostConflict(err)
	}

	if !updated {
		return fmt.Errorf("%w: %d", ErrHostNotFound, id)
	}

	recordAudit(ctx, h.audit, AuditTypeHostUpdated, auditHostUpdateData{
		Name: name, Address: address, OldName: host.Name, OldAddress: host.Address,
	})

	return nil
}

func hostConflict(err error) error {
	switch {
	case errors.Is(err, store.ErrHostNameConflict):
		return ErrHostAlreadyExists
	case errors.Is(err, store.ErrHostAddressConflict):
		return ErrHostAddressInUse
	}

	return err
}

func validateHost(name, address string) (string, string, error) {
	name = strings.TrimSpace(name)

	if name == "" {
		return "", "", fmt.Errorf("%w: name is required", ErrInvalidHost)
	}

	if len(name) > maxHostNameLength {
		return "", "", fmt.Errorf("%w: name must be at most %d characters", ErrInvalidHost, maxHostNameLength)
	}

	address, err := normalizeAgentAddress(address)

	if err != nil {
		return "", "", err
	}

	return name, address, nil
}

func normalizeAgentAddress(raw string) (string, error) {
	address := strings.TrimSpace(raw)

	if address == "" {
		return "", fmt.Errorf("%w: address is required", ErrInvalidHost)
	}

	if len(address) > maxHostAddressLength || strings.Contains(address, "://") || strings.ContainsAny(address, "/?#@ \t") {
		return "", fmt.Errorf("%w: address must be a host or host:port, like 10.0.0.5 or nas.lan:%s", ErrInvalidHost, agentapi.DefaultPort)
	}

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		host, port = strings.TrimSuffix(strings.TrimPrefix(address, "["), "]"), ""
	}

	if port == "" {
		port = agentapi.DefaultPort
	}

	if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("%w: port must be a number from 1 to 65535", ErrInvalidHost)
	}

	if host == "" || strings.ContainsAny(host, "[]") {
		return "", fmt.Errorf("%w: address must be a host or host:port, like 10.0.0.5 or nas.lan:%s", ErrInvalidHost, agentapi.DefaultPort)
	}

	return net.JoinHostPort(host, port), nil
}

func (h *Hosts) List(ctx context.Context, proxy ProxyTarget) ([]model.Host, error) {
	hosts, _, err := h.ListWithBehind(ctx, proxy)

	return hosts, err
}

func (h *Hosts) ListWithBehind(ctx context.Context, proxy ProxyTarget) ([]model.Host, []model.HostContainer, error) {
	hosts, containers, err := h.trackedContainers(ctx, proxy)

	if err != nil {
		return nil, nil, err
	}

	var behind []model.HostContainer

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

		if c.Tracking != nil && c.Tracking.NeedsUpdate() {
			behind = append(behind, c)
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

	slices.SortStableFunc(behind, func(a, b model.HostContainer) int {
		return cmp.Or(
			cmp.Compare(behindRank(a), behindRank(b)),
			strings.Compare(hosts[indexes[a.HostID]].Name, hosts[indexes[b.HostID]].Name),
			strings.Compare(a.Name, b.Name),
		)
	})

	return hosts, behind, nil
}

func behindRank(c model.HostContainer) int {
	if c.Tracking.Status == model.ContainerOutdated {
		return 0
	}

	return 1
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
		if c.Tracking == nil || !c.Tracking.InRollout || c.Tracking.ImageID == nil {
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

	if host.PollError == nil {
		if err := h.poll(ctx, id, livePollMaxAge, livePollTimeout); err != nil {
			return nil, err
		}

		if host, err = h.getHost(ctx, id); err != nil {
			return nil, err
		}
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

func (h *Hosts) TrustNewAgent(ctx context.Context, id int64) error {
	host, err := h.getHost(ctx, id)

	if err != nil {
		return err
	}

	if host.PollErrorKind == nil || *host.PollErrorKind != model.HostPollIdentityChanged {
		return fmt.Errorf("%w: the agent's identity has not changed", ErrInvalidHost)
	}

	if err := h.store.SetHostAgent(ctx, id, nil); err != nil {
		return err
	}

	recordAudit(ctx, h.audit, AuditTypeHostAgentTrusted, auditHostData{Name: host.Name, Address: host.Address})

	return h.poll(ctx, id, 0, livePollTimeout)
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

	h.mu.Lock()
	delete(h.polls, id)
	delete(h.polled, id)
	h.mu.Unlock()

	recordAudit(ctx, h.audit, AuditTypeHostDeleted, auditHostData{Name: host.Name, Address: host.Address})

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
	host.Online = host.LastSeenAt != nil && !host.AgentUpdateNeeded && !h.agentRefused(host) && h.now().Sub(*host.LastSeenAt) < h.offlineAfter
}

func (h *Hosts) agentRefused(host *model.Host) bool {
	return host.PollErrorKind != nil && *host.PollErrorKind != model.HostPollFailed
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

		if image == nil && h.internalRegistry != nil && h.internalRegistry.Address() != "" {
			h.trackInternal(c)
		}
	}

	return nil
}

func (h *Hosts) trackInternal(c *model.HostContainer) {
	c.Tracking.Status = model.ContainerInternalRegistry
	c.Tracking.InternalRegistry = true
	c.Tracking.PinnedTag = nil

	if c.ImageTag == nil || len(c.RunningDigests) == 0 {
		return
	}

	digest := h.internalRegistry.TagDigest(c.ImageRepository, *c.ImageTag)

	if digest == "" {
		return
	}

	c.Tracking.Status = versionStatus(*c, digest)
	c.Tracking.InRollout = c.Running() || c.Restarting()
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

	if digest == nil || len(c.RunningDigests) == 0 {
		tracking.Status = model.ContainerVersionUnknown
	} else {
		tracking.Status = versionStatus(c, *digest)
	}

	tracking.InRollout = (c.Running() || c.Restarting()) && followsVirtualTag && tracking.Status != model.ContainerVersionUnknown

	return tracking
}

func versionStatus(c model.HostContainer, digest string) model.ContainerVersionStatus {
	switch {
	case slices.Contains(c.RunningDigests, digest):
		return model.ContainerUpToDate
	case slices.Contains(c.RefDigests, digest):
		return model.ContainerPulledNotRecreated
	default:
		return model.ContainerOutdated
	}
}
