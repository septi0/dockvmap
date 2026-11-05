package agentapi

import "strings"

const ProtocolVersion = 1

const DefaultPort = "9002"

const (
	AdoptPath    = "/v1/adopt"
	SnapshotPath = "/v1/snapshot"
)

const ProxyHostHeader = "X-Dockvmap-Proxy-Host"

const (
	MaxContainers = 1000
	MaxImages     = 2000
	MaxTextLength = 255
)

type Adoption struct {
	ProtocolVersion int    `json:"protocolVersion"`
	AgentVersion    string `json:"agentVersion"`
}

type Error struct {
	Error string `json:"error"`
}

type Report struct {
	ProtocolVersion int         `json:"protocolVersion"`
	AgentVersion    string      `json:"agentVersion"`
	Fingerprint     string      `json:"fingerprint"`
	Host            HostInfo    `json:"host"`
	Containers      []Container `json:"containers"`
	Images          []Image     `json:"images"`
	DockerError     string      `json:"dockerError,omitempty"`
}

type HostInfo struct {
	Hostname      string `json:"hostname"`
	DockerVersion string `json:"dockerVersion"`
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	CPUs          int    `json:"cpus"`
	MemoryBytes   int64  `json:"memoryBytes"`
}

type Container struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Image             string `json:"image"`
	ImageID           string `json:"imageId"`
	RefImageID        string `json:"refImageId,omitempty"`
	State             string `json:"state"`
	Health            string `json:"health,omitempty"`
	ExitCode          int    `json:"exitCode"`
	ComposeProject    string `json:"composeProject,omitempty"`
	ComposeService    string `json:"composeService,omitempty"`
	ComposeWorkingDir string `json:"composeWorkingDir,omitempty"`
	Created           int64  `json:"created"`
	StartedAt         int64  `json:"startedAt,omitempty"`
	FinishedAt        int64  `json:"finishedAt,omitempty"`
	Ports             []Port `json:"ports,omitempty"`
}

type Port struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort int    `json:"privatePort"`
	PublicPort  int    `json:"publicPort"`
	Type        string `json:"type"`
}

type Image struct {
	ID          string   `json:"id"`
	RepoDigests []string `json:"repoDigests"`
}

type ImageRef struct {
	Host       string
	Repository string
	Tag        string
	Digest     string
}

func ParseImageRef(ref string) ImageRef {
	if strings.HasPrefix(ref, "sha256:") {
		return ImageRef{}
	}

	var parsed ImageRef

	name, digest, hasDigest := strings.Cut(ref, "@")

	if hasDigest {
		parsed.Digest = digest
	}

	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		parsed.Tag = name[colon+1:]
		name = name[:colon]
	}

	if name == "" {
		return ImageRef{}
	}

	first, rest, hasSlash := strings.Cut(name, "/")

	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		parsed.Host = strings.ToLower(first)
		parsed.Repository = rest
	} else {
		parsed.Host = "docker.io"
		parsed.Repository = name

		if !hasSlash {
			parsed.Repository = "library/" + name
		}
	}

	if parsed.Tag == "" && parsed.Digest == "" {
		parsed.Tag = "latest"
	}

	return parsed
}

func (r ImageRef) OnHost(host string) bool {
	if r.Host == "" || host == "" {
		return false
	}

	host = strings.ToLower(host)

	if r.Host == host {
		return true
	}

	bare, ok := strings.CutSuffix(host, ":443")

	return ok && r.Host == bare
}
