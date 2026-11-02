package model

import (
	"slices"
	"time"
)

var cleanExitCodes = []int{0, 137, 143}

const (
	HostPollFailed          = "failed"
	HostPollIdentityChanged = "identityChanged"
	HostPollPairedElsewhere = "pairedElsewhere"
)

type Host struct {
	ID                  int64
	Name                string
	Address             string
	AgentFingerprint    *string
	CreatedAt           time.Time
	LastSeenAt          *time.Time
	PollError           *string
	PollErrorKind       *string
	AgentVersion        *string
	ProtocolVersion     *int
	Info                HostInfo
	DockerError         *string
	ReportFingerprint   *string
	RunningContainers   int
	StoppedContainers   int
	Stacks              int
	UnhealthyContainers int
	CrashedContainers   int
	ProxyContainers     int
	Rollout             Rollout
	LastStartedAt       *time.Time
	Online              bool
	AgentUpdateNeeded   bool
}

func (h Host) Current() bool {
	return h.Online && h.DockerError == nil
}

type HostInfo struct {
	Hostname      *string
	DockerVersion *string
	OS            *string
	Architecture  *string
	CPUs          *int
	MemoryBytes   *int64
}

type HostSnapshot struct {
	AgentVersion    string
	ProtocolVersion int
	Info            HostInfo
	DockerError     *string
	Containers      []HostContainer
	Fingerprint     string
}

type HostContainer struct {
	HostID          int64
	ContainerID     string
	Name            string
	Image           string
	ImageHost       string
	ImageRepository string
	ImageTag        *string
	ImageDigest     *string
	RunningDigests  []string
	RefDigests      []string
	State           string
	Health          *string
	ExitCode        int
	ComposeProject  *string
	ComposeService  *string
	CreatedAt       *time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	Ports           []ContainerPort
	Tracking        *ContainerTracking
	RecentlyCrashed bool
}

func (c HostContainer) Running() bool {
	return c.State == "running"
}

func (c HostContainer) Restarting() bool {
	return c.State == "restarting"
}

func (c HostContainer) Crashed() bool {
	if c.State == "dead" || c.Restarting() {
		return true
	}

	return c.State == "exited" && !slices.Contains(cleanExitCodes, c.ExitCode)
}

type ContainerPort struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort"`
	Type        string `json:"type"`
}

type ContainerVersionStatus string

const (
	ContainerUpToDate           ContainerVersionStatus = "upToDate"
	ContainerOutdated           ContainerVersionStatus = "outdated"
	ContainerPulledNotRecreated ContainerVersionStatus = "pulledNotRecreated"
	ContainerVersionUnknown     ContainerVersionStatus = "unknown"
	ContainerUnknownImage       ContainerVersionStatus = "unknownImage"
)

type ContainerTracking struct {
	ImageID   *int64
	ImageName string
	Status    ContainerVersionStatus
	PinnedTag *string
	InRollout bool
}

func (t ContainerTracking) NeedsUpdate() bool {
	return t.InRollout && t.Status != ContainerUpToDate
}

type Rollout struct {
	UpToDate int
	Total    int
}

func (r *Rollout) Add(tracking *ContainerTracking) {
	if tracking == nil || !tracking.InRollout {
		return
	}

	r.Total++

	if tracking.Status == ContainerUpToDate {
		r.UpToDate++
	}
}

func (r Rollout) Outdated() int {
	return r.Total - r.UpToDate
}

type ImageRollout struct {
	ImageID          int64
	Rollout          Rollout
	HostsOffline     int
	HostsDockerError int
}

type HostDetails struct {
	Host       Host
	Containers []HostContainer
}

type ImageContainers struct {
	Hosts           []HostDetails
	Rollout         Rollout
	ConfiguredHosts int
}
