package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/agentclient"
	"github.com/septi0/dockvmap/internal/model"
)

const proxyHost = "proxy.test:5050"

var testProxy = ProxyTarget{Host: proxyHost, VirtualTag: "current"}

type fakeAgent struct {
	fingerprint     string
	report          *agentapi.Report
	err             error
	pairedElsewhere bool
	adopted         bool
	calls           int
	adoptions       int
	snapshots       int
	notModified     int
	proxyHost       string
}

type fakeAgents struct {
	mu     sync.Mutex
	agents map[string]*fakeAgent
}

func (f *fakeAgents) get(address, pinned string) (*fakeAgent, error) {
	agent, ok := f.agents[address]

	if !ok {
		return nil, errors.New("dial tcp " + address + ": no such host")
	}

	agent.calls++

	switch {
	case agent.err != nil:
		return nil, agent.err
	case pinned != "" && pinned != agent.fingerprint:
		return nil, agentclient.ErrIdentityChanged
	case agent.pairedElsewhere:
		return nil, agentclient.ErrPairedElsewhere
	}

	return agent, nil
}

func (f *fakeAgents) Adopt(_ context.Context, address, pinned string) (agentapi.Adoption, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	agent, err := f.get(address, pinned)

	if err != nil {
		return agentapi.Adoption{}, "", err
	}

	agent.adopted = true
	agent.adoptions++

	return agentapi.Adoption{ProtocolVersion: agentapi.ProtocolVersion, AgentVersion: "v1.9.0"}, agent.fingerprint, nil
}

func (f *fakeAgents) Snapshot(_ context.Context, address, pinned, fingerprint, proxyHost string) (*agentapi.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	agent, err := f.get(address, pinned)

	if err != nil {
		return nil, err
	}

	if !agent.adopted {
		return nil, agentclient.ErrNotAdopted
	}

	agent.snapshots++
	agent.proxyHost = proxyHost

	if fingerprint != "" && agent.report.Fingerprint == fingerprint {
		agent.notModified++
		return nil, nil
	}

	report := *agent.report

	return &report, nil
}

func (f *fakeAgents) agent(address string) *fakeAgent {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.agents[address]
}

func (f *fakeAgents) change(address string, change func(*fakeAgent)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.agents[address])
}

type hostFixture struct {
	*groupFixture
	hosts  *Hosts
	agents *fakeAgents
	usage  *ProxyUsage
	clock  *stepClock
}

func newHostFixture(t *testing.T) *hostFixture {
	f := newGroupFixture(t)
	clock := &stepClock{at: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	agents := &fakeAgents{agents: make(map[string]*fakeAgent)}
	usage := NewProxyUsage(f.store)
	hosts := NewHosts(HostsDeps{Store: f.store, Audit: f.audit, Agents: agents, Usage: usage, PollInterval: time.Minute, ProxyHost: proxyHost})
	hosts.now = clock.now

	return &hostFixture{groupFixture: f, hosts: hosts, agents: agents, usage: usage, clock: clock}
}

func (f *hostFixture) addHost(name string) (int64, string) {
	f.t.Helper()

	address := strings.TrimSpace(name) + ".lan:9002"

	id, err := f.hosts.Create(f.ctx, name, strings.TrimSpace(name)+".lan")

	if err != nil {
		f.t.Fatalf("creating host %q: %v", name, err)
	}

	f.agents.mu.Lock()
	f.agents.agents[address] = &fakeAgent{fingerprint: "sha256:" + name, report: &agentapi.Report{}, err: errors.New("dial tcp " + address + ": connect: connection refused")}
	f.agents.mu.Unlock()

	return id, address
}

func (f *hostFixture) host(id int64) *model.Host {
	f.t.Helper()

	host, err := f.hosts.getHost(f.ctx, id)

	if err != nil {
		f.t.Fatalf("getting host %d: %v", id, err)
	}

	return host
}

func (f *hostFixture) poll(id int64) {
	f.t.Helper()

	if err := f.hosts.poll(f.ctx, id, 0, time.Second); err != nil {
		f.t.Fatalf("poll host %d: %v", id, err)
	}
}

func (f *hostFixture) send(id int64, report agentapi.Report) {
	f.t.Helper()

	f.agents.change(f.host(id).Address, func(a *fakeAgent) { a.report, a.err = &report, nil })
	f.poll(id)

	if host := f.host(id); host.PollError != nil {
		f.t.Fatalf("poll host %d failed: %s", id, *host.PollError)
	}
}

func (f *hostFixture) report(id int64, containers []agentapi.Container, images []agentapi.Image) {
	f.t.Helper()

	f.send(id, agentapi.Report{
		ProtocolVersion: agentapi.ProtocolVersion,
		AgentVersion:    "v1.9.0",
		Fingerprint:     testFingerprint(containers, images),
		Host:            agentapi.HostInfo{Hostname: "srv", DockerVersion: "29.0.1", OS: "Debian", Architecture: "x86_64", CPUs: 4, MemoryBytes: 8 << 30},
		Containers:      containers,
		Images:          images,
	})
}

func (f *hostFixture) dockerError(id int64) {
	f.t.Helper()

	f.send(id, agentapi.Report{
		ProtocolVersion: agentapi.ProtocolVersion,
		AgentVersion:    "v1.9.0",
		Fingerprint:     "docker-error",
		DockerError:     "cannot read Docker at unix:///var/run/docker.sock: permission denied",
	})
}

func (f *hostFixture) apply(id int64, report agentapi.Report) error {
	return f.hosts.apply(f.ctx, f.host(id), &report)
}

func (f *hostFixture) unreachable(id int64) {
	f.agents.change(f.host(id).Address, func(a *fakeAgent) { a.err = errors.New("i/o timeout") })
}

func (f *hostFixture) setDigest(imageID int64, tag, digest string) {
	f.t.Helper()

	if err := f.store.UpdateImageDigest(f.ctx, imageID, tag, &digest, f.clock.at); err != nil {
		f.t.Fatalf("setting digest: %v", err)
	}
}

func testFingerprint(containers []agentapi.Container, images []agentapi.Image) string {
	encoded, _ := json.Marshal([]any{containers, images})
	sum := sha256.Sum256(encoded)

	return hex.EncodeToString(sum[:])
}

func container(id, name, image, imageID string) agentapi.Container {
	return agentapi.Container{ID: id, Name: name, Image: image, ImageID: imageID, State: "running", Created: 1_700_000_000, StartedAt: 1_700_000_100,
		Ports: []agentapi.Port{{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"}}}
}

func repoDigest(repo, c string) string { return repo + "@" + testDigest(c) }

func TestHostCreateValidatesNameAndAddress(t *testing.T) {
	f := newHostFixture(t)

	id, _ := f.addHost("  server2  ")

	if host := f.host(id); host.Name != "server2" || host.Address != "server2.lan:9002" || host.AgentFingerprint != nil {
		t.Fatalf("host = %+v", host)
	}

	if _, err := f.hosts.Create(f.ctx, "server2", "other.lan"); !errors.Is(err, ErrHostAlreadyExists) {
		t.Fatalf("duplicate name err = %v", err)
	}

	if _, err := f.hosts.Create(f.ctx, "server2-again", "server2.lan:9002"); !errors.Is(err, ErrHostAddressInUse) {
		t.Fatalf("duplicate address err = %v", err)
	}

	for _, bad := range []string{" ", strings.Repeat("a", 101)} {
		if _, err := f.hosts.Create(f.ctx, bad, "srv.lan"); !errors.Is(err, ErrInvalidHost) {
			t.Fatalf("name %q err = %v", bad, err)
		}
	}

	addresses := map[string]string{
		"10.0.0.5":             "10.0.0.5:9002",
		" nas.lan:9100 ":       "nas.lan:9100",
		"nas.lan:":             "nas.lan:9002",
		"fd00::5":              "[fd00::5]:9002",
		"[fd00::5]":            "[fd00::5]:9002",
		"[fd00::5]:9100":       "[fd00::5]:9100",
		"core1.uscrib.ro:9002": "core1.uscrib.ro:9002",
	}

	for raw, want := range addresses {
		if got, err := normalizeAgentAddress(raw); err != nil || got != want {
			t.Errorf("address %q = %q, %v, want %q", raw, got, err, want)
		}
	}

	for _, bad := range []string{"", "https://nas.lan", "nas.lan/agent", "nas.lan:0", "nas.lan:70000", "nas.lan:http", "user@nas.lan", "nas lan", ":9002"} {
		if _, err := normalizeAgentAddress(bad); !errors.Is(err, ErrInvalidHost) {
			t.Errorf("address %q err = %v", bad, err)
		}
	}

	if f.audit.count(AuditTypeHostCreated) != 1 {
		t.Fatalf("created audit rows = %d", f.audit.count(AuditTypeHostCreated))
	}
}

func TestHostUpdateChangesNameAndAddress(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")
	f.addHost("server3")
	f.report(id, nil, nil)

	if err := f.hosts.Update(f.ctx, id, "server2", "server2.lan"); err != nil || f.audit.count(AuditTypeHostUpdated) != 0 {
		t.Fatalf("an unchanged update = %v, audited %d times", err, f.audit.count(AuditTypeHostUpdated))
	}

	if err := f.hosts.Update(f.ctx, id, "web", "10.0.0.7:9100"); err != nil {
		t.Fatalf("update: %v", err)
	}

	if host := f.host(id); host.Name != "web" || host.Address != "10.0.0.7:9100" || host.AgentFingerprint == nil {
		t.Fatalf("host = %+v, want the new name and address and the same agent", host)
	}

	if err := f.hosts.Update(f.ctx, id, "server3", "srv.lan"); !errors.Is(err, ErrHostAlreadyExists) {
		t.Fatalf("taken name err = %v", err)
	}

	if err := f.hosts.Update(f.ctx, id, "web", "server3.lan"); !errors.Is(err, ErrHostAddressInUse) {
		t.Fatalf("taken address err = %v", err)
	}

	if err := f.hosts.Update(f.ctx, id, "web", "https://srv"); !errors.Is(err, ErrInvalidHost) {
		t.Fatalf("bad address err = %v", err)
	}

	if err := f.hosts.Update(f.ctx, 999, "x", "x.lan"); !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("missing host err = %v", err)
	}

	if f.audit.count(AuditTypeHostUpdated) != 1 {
		t.Fatal("update not audited")
	}
}

func TestHostPollAdoptsThenOnlyAsksForChanges(t *testing.T) {
	f := newHostFixture(t)
	id, address := f.addHost("server2")

	f.report(id, []agentapi.Container{container("c1", "web-1", "nginx", "img1")}, nil)

	agent := f.agents.agent(address)
	host := f.host(id)

	if agent.adoptions != 1 || host.AgentFingerprint == nil || *host.AgentFingerprint != "sha256:server2" || agent.proxyHost != proxyHost {
		t.Fatalf("first poll should adopt and pin the agent: adoptions %d, host %+v, proxy %q", agent.adoptions, host.AgentFingerprint, agent.proxyHost)
	}

	seen := *host.LastSeenAt

	f.clock.advance(time.Minute)
	f.poll(id)

	if agent.adoptions != 1 || agent.notModified != 1 || !f.host(id).LastSeenAt.After(seen) {
		t.Fatalf("a repeat poll should ask only for changes: adoptions %d, not modified %d", agent.adoptions, agent.notModified)
	}

	f.agents.change(address, func(a *fakeAgent) { a.adopted = false })
	f.poll(id)

	if agent.adoptions != 2 || f.host(id).PollError != nil {
		t.Fatalf("an agent that lost its pairing but kept its identity is adopted again: adoptions %d, error %v", agent.adoptions, f.host(id).PollError)
	}
}

func TestHostPollRefusesAChangedIdentityUntilTrusted(t *testing.T) {
	f := newHostFixture(t)
	id, address := f.addHost("server2")
	f.report(id, []agentapi.Container{container("c1", "web-1", "nginx", "img1")}, nil)

	if err := f.hosts.TrustNewAgent(f.ctx, id); !errors.Is(err, ErrInvalidHost) {
		t.Fatalf("trust without an identity change err = %v", err)
	}

	f.agents.change(address, func(a *fakeAgent) { a.fingerprint, a.adopted = "sha256:impostor", false })
	f.poll(id)

	host := f.host(id)

	if host.PollErrorKind == nil || *host.PollErrorKind != model.HostPollIdentityChanged || host.Online || *host.AgentFingerprint != "sha256:server2" {
		t.Fatalf("host = %+v, want the old pin kept, the change flagged and the host not online", host)
	}

	if details, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || len(details.Containers) != 1 {
		t.Fatalf("the last known containers stay visible: %+v, %v", details, err)
	}

	if err := f.hosts.TrustNewAgent(f.ctx, id); err != nil {
		t.Fatalf("trust: %v", err)
	}

	host = f.host(id)

	if host.AgentFingerprint == nil || *host.AgentFingerprint != "sha256:impostor" || host.PollError != nil || !host.Online {
		t.Fatalf("host = %+v, want the new agent adopted and polled", host)
	}

	if f.audit.count(AuditTypeHostAgentTrusted) != 1 {
		t.Fatal("trust not audited")
	}
}

func TestHostPollFailuresAreRecordedOnTheHost(t *testing.T) {
	f := newHostFixture(t)
	id, address := f.addHost("server2")

	f.poll(id)

	if host := f.host(id); host.PollError == nil || !strings.Contains(*host.PollError, "connection refused") || *host.PollErrorKind != model.HostPollFailed || host.Online {
		t.Fatalf("an unreachable new host = %+v", host)
	}

	f.report(id, nil, nil)
	f.unreachable(id)
	f.clock.advance(time.Minute)
	f.poll(id)

	if host := f.host(id); !host.Online || host.PollError == nil {
		t.Fatalf("one failed poll keeps the host online until the offline window: %+v", host)
	}

	f.agents.change(address, func(a *fakeAgent) { a.err, a.pairedElsewhere = nil, true })
	f.poll(id)

	if host := f.host(id); *host.PollErrorKind != model.HostPollPairedElsewhere || host.Online {
		t.Fatalf("an agent paired elsewhere = %+v", host)
	}

	f.agents.change(address, func(a *fakeAgent) { a.pairedElsewhere = false })
	f.poll(id)

	if host := f.host(id); host.PollError != nil || host.PollErrorKind != nil || !host.Online {
		t.Fatalf("a good poll clears the error: %+v", host)
	}
}

func TestHostPollAllPollsEveryHost(t *testing.T) {
	f := newHostFixture(t)
	up, _ := f.addHost("up")
	down, _ := f.addHost("down")
	f.unreachable(down)

	f.agents.change(f.host(up).Address, func(a *fakeAgent) {
		a.report, a.err = &agentapi.Report{ProtocolVersion: agentapi.ProtocolVersion, AgentVersion: "v1.9.0", Fingerprint: "fp"}, nil
	})

	polled, err := f.hosts.PollAll(f.ctx)

	if err != nil || polled != 2 {
		t.Fatalf("poll all = %d, %v; an unreachable host is recorded on the host, not returned", polled, err)
	}

	if !f.host(up).Online || f.host(down).PollError == nil {
		t.Fatalf("up = %+v, down = %+v", f.host(up), f.host(down))
	}
}

func TestHostGetPollsLiveUnlessTheHostIsFailing(t *testing.T) {
	f := newHostFixture(t)
	id, address := f.addHost("server2")
	f.report(id, nil, nil)
	agent := f.agents.agent(address)

	calls := agent.calls

	if _, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || agent.calls != calls {
		t.Fatalf("a poll a moment ago is fresh enough: calls %d, %v", agent.calls-calls, err)
	}

	f.clock.advance(livePollMaxAge)

	if _, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || agent.calls != calls+1 {
		t.Fatalf("opening the host polls it: calls %d, %v", agent.calls-calls, err)
	}

	f.clock.advance(livePollMaxAge)
	busy := f.hosts.pollLock(id)
	busy.Lock()

	if _, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || agent.calls != calls+1 {
		t.Fatalf("opening the host while it is being polled doesn't wait for that poll: calls %d, %v", agent.calls-calls, err)
	}

	busy.Unlock()

	f.unreachable(id)
	f.clock.advance(livePollMaxAge)

	if _, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || agent.calls != calls+2 || f.host(id).PollError == nil {
		t.Fatalf("a failed live poll is recorded: calls %d, %v", agent.calls-calls, err)
	}

	f.clock.advance(livePollMaxAge)

	if _, err := f.hosts.Get(f.ctx, id, testProxy); err != nil || agent.calls != calls+2 {
		t.Fatalf("a failing host is left to the background poll: calls %d, %v", agent.calls-calls, err)
	}
}

func TestHostReportStoresSnapshotAndSkipsUnchangedOnes(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	containers := []agentapi.Container{
		container("c1", "web-1", "nginx:1.27", "img1"),
		{ID: "c2", Name: "old-job", Image: "busybox", ImageID: "img2", State: "exited", ExitCode: 3, StartedAt: 1_700_000_000, FinishedAt: 1_700_000_500},
	}

	f.report(id, containers, nil)

	details, err := f.hosts.Get(f.ctx, id, testProxy)

	if err != nil || !details.Host.Online || len(details.Containers) != 2 {
		t.Fatalf("details = %+v, err = %v", details, err)
	}

	if details.Host.RunningContainers != 1 || details.Host.StoppedContainers != 1 {
		t.Fatalf("counts = %d running, %d stopped", details.Host.RunningContainers, details.Host.StoppedContainers)
	}

	if *details.Host.Info.DockerVersion != "29.0.1" || *details.Host.Info.CPUs != 4 || *details.Host.AgentVersion != "v1.9.0" {
		t.Fatalf("host info = %+v", details.Host.Info)
	}

	for _, c := range details.Containers {
		if c.Name == "old-job" && (c.ExitCode != 3 || c.StartedAt == nil || c.FinishedAt == nil || c.FinishedAt.Unix() != 1_700_000_500) {
			t.Fatalf("stopped container times = %+v", c)
		}

		if c.Name == "web-1" && (c.StartedAt == nil || c.FinishedAt != nil || len(c.Ports) != 1 || c.Ports[0].PublicPort != 8080) {
			t.Fatalf("running container times = %+v", c)
		}
	}

	firstSeen := *details.Host.LastSeenAt
	fingerprint := *details.Host.ReportFingerprint

	f.clock.advance(time.Minute)
	f.report(id, containers, nil)

	again, _ := f.hosts.getHost(f.ctx, id)

	if *again.ReportFingerprint != fingerprint || !again.LastSeenAt.After(firstSeen) {
		t.Fatalf("an unchanged report should only touch the host: fingerprint %v, seen %v", *again.ReportFingerprint, again.LastSeenAt)
	}

	containers[0].State, containers[0].ExitCode = "exited", 137
	f.report(id, containers, nil)

	changed, _ := f.hosts.Get(f.ctx, id, testProxy)

	if *changed.Host.ReportFingerprint == fingerprint || changed.Host.RunningContainers != 0 || changed.Host.StoppedContainers != 2 {
		t.Fatalf("changed report not stored: %+v", changed.Host)
	}
}

func TestHostGoesDownAfterThreeMissedPolls(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	if details, _ := f.hosts.Get(f.ctx, id, testProxy); details.Host.Online {
		t.Fatal("a host that never answered is online")
	}

	f.report(id, []agentapi.Container{container("c1", "web-1", "nginx", "img1")}, nil)
	f.unreachable(id)

	f.clock.advance(f.hosts.offlineAfter - time.Second)

	if details, _ := f.hosts.Get(f.ctx, id, testProxy); !details.Host.Online || len(details.Containers) != 1 {
		t.Fatalf("host should still be online: %+v", details)
	}

	f.clock.advance(2 * time.Second)

	details, _ := f.hosts.Get(f.ctx, id, testProxy)

	if details.Host.Online || len(details.Containers) != 1 || details.Host.RunningContainers != 1 {
		t.Fatalf("a down host keeps its last known containers: %+v", details)
	}
}

func TestHostReportRejectsUnsupportedProtocol(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	err := f.apply(id, agentapi.Report{ProtocolVersion: agentapi.ProtocolVersion + 1, AgentVersion: "v9.0.0"})

	if !errors.Is(err, ErrUnsupportedAgentProtocol) {
		t.Fatalf("err = %v", err)
	}

	host, _ := f.hosts.getHost(f.ctx, id)

	if host.Online || host.LastSeenAt != nil || *host.ProtocolVersion != agentapi.ProtocolVersion+1 || *host.AgentVersion != "v9.0.0" {
		t.Fatalf("host = %+v, want versions recorded and still offline", host)
	}

	f.report(id, nil, nil)

	if host, _ := f.hosts.getHost(f.ctx, id); !host.Online {
		t.Fatal("a supported report should bring the host online")
	}
}

func TestHostReportValidation(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	valid := container("c1", "web-1", "nginx", "img1")

	mutate := func(change func(*agentapi.Container)) []agentapi.Container {
		c := valid
		change(&c)

		return []agentapi.Container{c}
	}

	cases := map[string]agentapi.Report{
		"missing state":      {Containers: mutate(func(c *agentapi.Container) { c.State = "" })},
		"long state":         {Containers: mutate(func(c *agentapi.Container) { c.State = strings.Repeat("s", 33) })},
		"unknown health":     {Containers: mutate(func(c *agentapi.Container) { c.Health = "meh" })},
		"missing id":         {Containers: mutate(func(c *agentapi.Container) { c.ID = "" })},
		"missing name":       {Containers: mutate(func(c *agentapi.Container) { c.Name = "" })},
		"long image":         {Containers: mutate(func(c *agentapi.Container) { c.Image = strings.Repeat("a", 513) })},
		"long compose label": {Containers: mutate(func(c *agentapi.Container) { c.ComposeProject = strings.Repeat("a", 256) })},
		"negative start":     {Containers: mutate(func(c *agentapi.Container) { c.StartedAt = -1 })},
		"port out of range": {Containers: mutate(func(c *agentapi.Container) {
			c.Ports = []agentapi.Port{{PrivatePort: 80, PublicPort: 70000, Type: "tcp"}}
		})},
		"unknown port type": {Containers: mutate(func(c *agentapi.Container) {
			c.Ports = []agentapi.Port{{PrivatePort: 80, PublicPort: 80, Type: "icmp"}}
		})},
		"too many ports":      {Containers: mutate(func(c *agentapi.Container) { c.Ports = make([]agentapi.Port, 101) })},
		"long fingerprint":    {Fingerprint: strings.Repeat("f", 129)},
		"missing fingerprint": {Fingerprint: "-"},
		"duplicate id":        {Containers: []agentapi.Container{valid, valid}},
		"too many":            {Containers: make([]agentapi.Container, agentapi.MaxContainers+1)},
		"negative resources":  {Host: agentapi.HostInfo{CPUs: -1}},
		"long host field":     {Host: agentapi.HostInfo{OS: strings.Repeat("a", 256)}},
		"image without id":    {Images: []agentapi.Image{{ID: ""}}},
		"long agent version":  {AgentVersion: strings.Repeat("v", 65)},
		"long docker error":   {DockerError: strings.Repeat("e", 256)},
	}

	for name, report := range cases {
		report.ProtocolVersion = agentapi.ProtocolVersion

		switch report.Fingerprint {
		case "":
			report.Fingerprint = "fp"
		case "-":
			report.Fingerprint = ""
		}

		if err := f.apply(id, report); !errors.Is(err, ErrInvalidAgentReport) {
			t.Errorf("%s: err = %v", name, err)
		}
	}

	if host, _ := f.hosts.getHost(f.ctx, id); host.LastSeenAt != nil {
		t.Fatal("rejected reports must not mark the host as seen")
	}
}

func TestHostDeleteRemovesContainers(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")
	webID := f.addImage("web", "v2", "v1", "v2")

	f.report(id, []agentapi.Container{container("c1", "web-1", proxyHost+"/web:current", "img1")}, nil)

	if err := f.hosts.Delete(f.ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if result, _ := f.hosts.ImageContainers(f.ctx, webID, testProxy); len(result.Hosts) != 0 {
		t.Fatalf("containers of a deleted host remain: %+v", result.Hosts)
	}

	if err := f.hosts.Delete(f.ctx, id); !errors.Is(err, ErrHostNotFound) {
		t.Fatalf("second delete err = %v", err)
	}

	if f.audit.count(AuditTypeHostDeleted) != 1 {
		t.Fatal("delete not audited")
	}
}

func TestHostContainerTracking(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")
	webID := f.addImage("web", "v2", "v1", "v2")
	f.setDigest(webID, "v2", testDigest("b"))
	f.addImage("nodigest", "v1", "v1")

	web := proxyHost + "/web"

	containers := []agentapi.Container{
		container("c1", "web-current", web+":current", "new"),
		container("c2", "web-old", web+":current", "old"),
		{ID: "c3", Name: "web-pulled", Image: web + ":current", ImageID: "old", RefImageID: "new", State: "running"},
		container("c4", "web-pinned", web+":v1", "old"),
		container("c5", "ghost", proxyHost+"/missing:current", "new"),
		container("c6", "plain-nginx", "nginx:1.27", "nginx"),
		container("c7", "other-address", "10.0.0.5:5050/web:current", "new"),
		container("c8", "no-digest", proxyHost+"/nodigest:current", "new"),
		container("c9", "local-build", web+":current", "local"),
		{ID: "c10", Name: "web-stopped", Image: web + ":current", ImageID: "new", State: "exited"},
	}

	images := []agentapi.Image{
		{ID: "new", RepoDigests: []string{repoDigest(web, "b")}},
		{ID: "old", RepoDigests: []string{repoDigest(web, "a"), "not-a-digest", web + "@sha256:XYZ"}},
		{ID: "nginx", RepoDigests: []string{repoDigest("nginx", "c")}},
	}

	f.report(id, containers, images)

	details, err := f.hosts.Get(f.ctx, id, testProxy)

	if err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		status model.ContainerVersionStatus
		pinned string
	}{
		"web-current": {model.ContainerUpToDate, ""},
		"web-old":     {model.ContainerOutdated, ""},
		"web-pulled":  {model.ContainerPulledNotRecreated, ""},
		"web-pinned":  {model.ContainerOutdated, "v1"},
		"ghost":       {model.ContainerUnknownImage, ""},
		"no-digest":   {model.ContainerVersionUnknown, ""},
		"local-build": {model.ContainerVersionUnknown, ""},
		"web-stopped": {model.ContainerUpToDate, ""},
	}

	for _, c := range details.Containers {
		expected, tracked := want[c.Name]

		if !tracked {
			if c.Tracking != nil {
				t.Errorf("%s should not be tracked: %+v", c.Name, c.Tracking)
			}

			continue
		}

		if c.Tracking == nil || c.Tracking.Status != expected.status {
			t.Errorf("%s tracking = %+v, want %s", c.Name, c.Tracking, expected.status)
			continue
		}

		pinned := ""

		if c.Tracking.PinnedTag != nil {
			pinned = *c.Tracking.PinnedTag
		}

		if pinned != expected.pinned {
			t.Errorf("%s pinned = %q, want %q", c.Name, pinned, expected.pinned)
		}
	}

	if len(details.Containers) != len(containers) {
		t.Fatalf("containers = %d, want %d", len(details.Containers), len(containers))
	}

	if err := f.images.UpdateTag(f.ctx, webID, "v1", model.TagHistorySourceManual); err != nil {
		t.Fatal(err)
	}

	retagged, _ := f.hosts.Get(f.ctx, id, testProxy)

	for _, c := range retagged.Containers {
		if c.Name == "web-current" && c.Tracking.Status != model.ContainerVersionUnknown {
			t.Fatalf("a digest of the previous tag must not count: %+v", c.Tracking)
		}
	}
}

func TestHostContainerTrackingUsesTheDigestTheProxyJustServed(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")
	webID := f.addImage("web", "v2", "v1", "v2")
	f.setDigest(webID, "v2", testDigest("a"))

	web := proxyHost + "/web"
	f.report(id,
		[]agentapi.Container{container("c1", "web", web+":current", "rebuilt")},
		[]agentapi.Image{{ID: "rebuilt", RepoDigests: []string{repoDigest(web, "b")}}},
	)

	status := func() model.ContainerVersionStatus {
		t.Helper()

		details, err := f.hosts.Get(f.ctx, id, testProxy)

		if err != nil || len(details.Containers) != 1 || details.Containers[0].Tracking == nil {
			t.Fatalf("details = %+v, err = %v", details, err)
		}

		return details.Containers[0].Tracking.Status
	}

	if got := status(); got != model.ContainerOutdated {
		t.Fatalf("before the pull: %s, want outdated", got)
	}

	f.usage.RecordTagDigest(webID, "v2", testDigest("b"))

	if got := status(); got != model.ContainerUpToDate {
		t.Fatalf("after the pull: %s, want upToDate", got)
	}
}

func TestHostContainerTrackingOnContainerdImageStore(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")
	webID := f.addImage("web", "v2", "v1", "v2")
	f.setDigest(webID, "v2", testDigest("b"))

	web := proxyHost + "/web"

	f.report(id, []agentapi.Container{
		{ID: "c1", Name: "web-pulled", Image: web + ":current", ImageID: testDigest("a"), RefImageID: testDigest("b"), State: "running"},
		{ID: "c2", Name: "web-old", Image: web + ":current", ImageID: testDigest("a"), State: "running"},
	}, []agentapi.Image{
		{ID: testDigest("b"), RepoDigests: []string{repoDigest(web, "b")}},
	})

	rolledBackID, _ := f.addHost("server3")

	f.report(rolledBackID, []agentapi.Container{
		{ID: "c3", Name: "web-rolled-back", Image: web + ":current", ImageID: testDigest("b"), RefImageID: testDigest("c"), State: "running"},
	}, []agentapi.Image{
		{ID: testDigest("c"), RepoDigests: []string{repoDigest(web, "c")}},
	})

	want := map[string]model.ContainerVersionStatus{
		"web-pulled":      model.ContainerPulledNotRecreated,
		"web-old":         model.ContainerOutdated,
		"web-rolled-back": model.ContainerUpToDate,
	}

	for _, hostID := range []int64{id, rolledBackID} {
		details, err := f.hosts.Get(f.ctx, hostID, testProxy)

		if err != nil {
			t.Fatal(err)
		}

		for _, c := range details.Containers {
			if c.Tracking == nil || c.Tracking.Status != want[c.Name] {
				t.Errorf("%s tracking = %+v, want %s", c.Name, c.Tracking, want[c.Name])
			}
		}
	}
}

func TestImageContainersGroupsByHost(t *testing.T) {
	f := newHostFixture(t)
	webID := f.addImage("web", "v2", "v2")
	f.setDigest(webID, "v2", testDigest("b"))
	f.addImage("api", "v1", "v1")

	web := proxyHost + "/web"
	images := []agentapi.Image{{ID: "new", RepoDigests: []string{repoDigest(web, "b")}}}

	alpha, _ := f.addHost("alpha")
	beta, _ := f.addHost("beta")
	down, _ := f.addHost("down")
	idle, _ := f.addHost("idle")

	f.report(down, []agentapi.Container{container("d1", "web", web+":current", "new")}, images)
	f.clock.advance(f.hosts.offlineAfter + time.Minute)

	f.report(beta, []agentapi.Container{
		container("b2", "web-2", web+":current", "new"),
		container("b1", "web-1", web+":current", "new"),
	}, images)
	f.report(alpha, []agentapi.Container{container("a1", "web", web+":current", "new")}, images)
	f.report(idle, []agentapi.Container{container("i1", "api", proxyHost+"/api:current", "x")}, nil)

	result, err := f.hosts.ImageContainers(f.ctx, webID, testProxy)

	if err != nil {
		t.Fatal(err)
	}

	entries := result.Hosts

	if result.ConfiguredHosts != 4 || len(entries) != 3 || entries[0].Host.Name != "alpha" || entries[1].Host.Name != "beta" || entries[2].Host.Name != "down" {
		t.Fatalf("hosts = %+v", entries)
	}

	if len(entries[1].Containers) != 2 || entries[1].Containers[0].Name != "web-1" || entries[1].Containers[1].Name != "web-2" {
		t.Fatalf("beta containers = %+v", entries[1].Containers)
	}

	if entries[2].Host.Online || len(entries[2].Containers) != 1 || result.Rollout.Total != 3 {
		t.Fatalf("a down host keeps its last known containers but isn't counted: %+v, rollout %+v", entries[2], result.Rollout)
	}

	if _, err := f.hosts.ImageContainers(f.ctx, 999, testProxy); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("missing image err = %v", err)
	}
}

func TestHostReportCarriesDockerErrors(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	f.report(id, []agentapi.Container{container("c1", "web-1", "nginx", "img1")}, nil)

	f.dockerError(id)

	details, _ := f.hosts.Get(f.ctx, id, testProxy)

	if !details.Host.Online || details.Host.Current() || details.Host.DockerError == nil || len(details.Containers) != 1 {
		t.Fatalf("details = %+v, want an online host with the error and its last known containers", details)
	}

	if details.Host.Info.DockerVersion == nil {
		t.Fatal("a Docker failure must keep the last known host info")
	}

	if hosts, _ := f.hosts.List(f.ctx, testProxy); hosts[0].Rollout.Total != 0 || hosts[0].LastStartedAt != nil {
		t.Fatalf("stale containers must not count: %+v", hosts[0])
	}

	f.report(id, []agentapi.Container{container("c1", "web-1", "nginx", "img1")}, nil)

	if details, _ := f.hosts.Get(f.ctx, id, testProxy); details.Host.DockerError != nil || len(details.Containers) != 1 {
		t.Fatalf("a good report must clear the error: %+v", details.Host.DockerError)
	}
}

func TestHostListCountsStacksHealthAndNeededActions(t *testing.T) {
	f := newHostFixture(t)
	webID := f.addImage("web", "v2", "v1", "v2")
	f.setDigest(webID, "v2", testDigest("b"))

	web := proxyHost + "/web"
	images := []agentapi.Image{
		{ID: "new", RepoDigests: []string{repoDigest(web, "b")}},
		{ID: "old", RepoDigests: []string{repoDigest(web, "a")}},
	}

	inStack := func(c agentapi.Container, project string) agentapi.Container {
		c.ComposeProject = project
		return c
	}

	sick := container("c4", "db", "postgres:16", "pg")
	sick.Health = "unhealthy"

	busy, _ := f.addHost("busy")
	f.report(busy, []agentapi.Container{
		inStack(container("c1", "web-current", web+":current", "new"), "shop"),
		inStack(container("c2", "web-old", web+":current", "old"), "shop"),
		inStack(container("c3", "web-pinned", web+":v1", "old"), "admin"),
		sick,
	}, images)

	gone, _ := f.addHost("gone")
	f.report(gone, []agentapi.Container{container("g1", "web-old", web+":current", "old")}, images)
	f.clock.advance(f.hosts.offlineAfter + time.Minute)
	f.report(busy, []agentapi.Container{
		inStack(container("c1", "web-current", web+":current", "new"), "shop"),
		inStack(container("c2", "web-old", web+":current", "old"), "shop"),
		inStack(container("c3", "web-pinned", web+":v1", "old"), "admin"),
		sick,
	}, images)

	hosts, err := f.hosts.List(f.ctx, testProxy)

	if err != nil || len(hosts) != 2 {
		t.Fatalf("hosts = %+v, err = %v", hosts, err)
	}

	byName := map[string]model.Host{hosts[0].Name: hosts[0], hosts[1].Name: hosts[1]}

	if b := byName["busy"]; b.Stacks != 2 || b.UnhealthyContainers != 1 || b.Rollout != (model.Rollout{UpToDate: 1, Total: 2}) || b.ProxyContainers != 3 {
		t.Fatalf("busy = stacks %d, unhealthy %d, rollout %+v, proxy %d; want 2, 1, 1 of 2, 3", b.Stacks, b.UnhealthyContainers, b.Rollout, b.ProxyContainers)
	}

	if g := byName["gone"]; g.Online || g.Rollout.Total != 0 || g.UnhealthyContainers != 0 {
		t.Fatalf("a down host must not report stale problems: %+v", g)
	}
}

func TestHostListCountsCrashesAndLastStart(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	exited := func(id string, code int) agentapi.Container {
		c := container(id, id, "nginx", "img1")
		c.State, c.ExitCode = "exited", code

		return c
	}

	newest := container("r2", "newest", "nginx", "img1")
	newest.StartedAt = 1_700_000_500

	crashedState := container("d1", "dead", "nginx", "img1")
	crashedState.State = "dead"

	looping := container("l1", "looping", "nginx", "img1")
	looping.State, looping.ExitCode = "restarting", 1

	oldCrash := exited("old", 1)
	oldCrash.FinishedAt = f.clock.now().Add(-8 * 24 * time.Hour).Unix()

	recentCrash := exited("e1", 1)
	recentCrash.FinishedAt = f.clock.now().Add(-6 * 24 * time.Hour).Unix()

	f.report(id, []agentapi.Container{
		container("r1", "older", "nginx", "img1"), newest, crashedState, looping, oldCrash, recentCrash,
		exited("e0", 0), exited("e137", 137), exited("e143", 143),
	}, nil)

	hosts, err := f.hosts.List(f.ctx, testProxy)

	if err != nil || len(hosts) != 1 {
		t.Fatalf("hosts = %+v, err = %v", hosts, err)
	}

	if hosts[0].CrashedContainers != 3 {
		t.Fatalf("crashed = %d, want the dead, restarting and recently crashed containers", hosts[0].CrashedContainers)
	}

	if hosts[0].LastStartedAt == nil || hosts[0].LastStartedAt.Unix() != newest.StartedAt {
		t.Fatalf("last started = %v, want %d", hosts[0].LastStartedAt, newest.StartedAt)
	}

	details, err := f.hosts.Get(f.ctx, id, testProxy)

	if err != nil {
		t.Fatal(err)
	}

	recent := 0

	for _, c := range details.Containers {
		if c.RecentlyCrashed {
			recent++
		}
	}

	if recent != hosts[0].CrashedContainers {
		t.Fatalf("recently crashed = %d, want the badge count %d", recent, hosts[0].CrashedContainers)
	}
}

func TestHostReportKeepsContainersInUnknownStates(t *testing.T) {
	f := newHostFixture(t)
	id, _ := f.addHost("server2")

	c := container("c1", "web", "nginx", "img1")
	c.State = "stopped"

	f.report(id, []agentapi.Container{c}, nil)

	if details, _ := f.hosts.Get(f.ctx, id, testProxy); len(details.Containers) != 1 || details.Containers[0].State != "stopped" {
		t.Fatalf("containers = %+v", details.Containers)
	}
}

func TestImageRolloutsCountKnownVersionsOnOnlineHosts(t *testing.T) {
	f := newHostFixture(t)
	webID := f.addImage("web", "v2", "v1", "v2")
	f.setDigest(webID, "v2", testDigest("b"))
	f.addImage("api", "v1", "v1")

	web := proxyHost + "/web"
	images := []agentapi.Image{
		{ID: "new", RepoDigests: []string{repoDigest(web, "b")}},
		{ID: "old", RepoDigests: []string{repoDigest(web, "a")}},
		{ID: "api", RepoDigests: []string{repoDigest(proxyHost+"/api", "c")}},
	}

	gone, _ := f.addHost("gone")
	f.report(gone, []agentapi.Container{container("g1", "web-old", web+":current", "old")}, images)
	f.clock.advance(f.hosts.offlineAfter + time.Minute)

	stopped := container("c8", "web-stopped", web+":current", "old")
	stopped.State = "exited"

	looping := container("c9", "web-looping", web+":current", "old")
	looping.State = "restarting"

	busy, _ := f.addHost("busy")
	f.report(busy, []agentapi.Container{
		container("c1", "web-a", web+":current", "new"),
		container("c2", "web-b", web+":current", "new"),
		container("c3", "web-old", web+":current", "old"),
		container("c4", "api", proxyHost+"/api:current", "api"),
		container("c5", "other", proxyHost+"/other:current", "old"),
		container("c6", "nginx", "nginx:1", "old"),
		container("c7", "web-pinned", web+":v1", "old"),
		stopped,
		looping,
	}, images)

	blind, _ := f.addHost("blind")
	f.report(blind, []agentapi.Container{container("d1", "web-blind", web+":current", "new")}, images)

	f.dockerError(blind)

	rollouts, err := f.hosts.ImageRollouts(f.ctx, testProxy)

	if err != nil {
		t.Fatalf("ImageRollouts: %v", err)
	}

	want := []model.ImageRollout{{ImageID: webID, Rollout: model.Rollout{UpToDate: 2, Total: 4}, HostsOffline: 1, HostsDockerError: 1}}

	if !slices.Equal(rollouts, want) {
		t.Fatalf("rollouts = %+v, want %+v (unknown digests, unknown images and pinned or stopped containers don't count; offline hosts and hosts that can't read Docker are counted apart)", rollouts, want)
	}
}

func TestHostContainerFromTheInternalRegistry(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		f := newHostFixture(t)
		f.hosts.internalRegistry = enabled
		id, _ := f.addHost("server2")

		f.report(id, []agentapi.Container{container("c1", "api", proxyHost+"/team/api:v3", "api")}, []agentapi.Image{
			{ID: "api", RepoDigests: []string{repoDigest(proxyHost+"/team/api", "a")}},
		})

		hosts, err := f.hosts.List(f.ctx, testProxy)

		if err != nil {
			t.Fatal(err)
		}

		details, err := f.hosts.Get(f.ctx, id, testProxy)

		if err != nil {
			t.Fatal(err)
		}

		want := model.ContainerUnknownImage

		if enabled {
			want = model.ContainerInternalRegistry
		}

		tracking := details.Containers[0].Tracking

		if tracking == nil || tracking.Status != want || tracking.InRollout {
			t.Errorf("internal registry %v: tracking = %+v, want %s outside the rollout", enabled, tracking, want)
		}

		if hosts[0].Rollout.Total != 0 {
			t.Errorf("internal registry %v: rollout = %+v, want nothing counted", enabled, hosts[0].Rollout)
		}
	}
}
