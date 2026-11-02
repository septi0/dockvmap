package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/agentclient"
)

const proxyHost = "registry.test:5050"

type fakeDocker struct {
	server *httptest.Server
	socket string
	events chan string
	minAPI string

	mu           sync.Mutex
	paths        []string
	methods      []string
	broken       bool
	hung         bool
	eventsBroken bool
	webState     string
}

func newFakeDocker(t *testing.T) *fakeDocker {
	t.Helper()

	d := &fakeDocker{
		socket:   filepath.Join(t.TempDir(), "docker.sock"),
		events:   make(chan string, 10),
		minAPI:   "1.24",
		webState: "running",
	}

	listener, err := net.Listen("unix", d.socket)

	if err != nil {
		t.Fatal(err)
	}

	d.server = httptest.NewUnstartedServer(http.HandlerFunc(d.serve))
	d.server.Listener = listener
	d.server.Start()
	t.Cleanup(d.server.Close)

	return d
}

func (d *fakeDocker) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	d.paths = append(d.paths, r.URL.Path)
	d.methods = append(d.methods, r.Method)
	broken, hung, eventsBroken, webState := d.broken, d.hung, d.eventsBroken, d.webState
	d.mu.Unlock()

	if hung && strings.HasSuffix(r.URL.Path, "/containers/json") {
		<-r.Context().Done()
		return
	}

	if broken && r.URL.Path != "/version" {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "daemon is sad"})

		return
	}

	version := "/v1.41"

	if d.minAPI != "1.24" {
		version = "/v" + d.minAPI
	}

	inspect := func(image, started, finished string, exitCode int) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Config": map[string]string{"Image": image},
			"State":  map[string]any{"StartedAt": started, "FinishedAt": finished, "ExitCode": exitCode},
		})
	}

	switch r.URL.Path {
	case "/version":
		_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.56", "MinAPIVersion": d.minAPI})

	case version + "/info":
		_ = json.NewEncoder(w).Encode(map[string]any{"Name": "srv2", "ServerVersion": "29.0.1", "OperatingSystem": "Debian", "Architecture": "x86_64", "NCPU": 4, "MemTotal": 8 << 30})

	case version + "/containers/json":
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "c1", "Names": []string{"/web-1"}, "Image": proxyHost + "/web:current", "ImageID": "sha256:old", "State": webState, "Status": "Up 2 hours (healthy)", "Created": 100,
				"Labels": map[string]string{"com.docker.compose.project": "shop", "com.docker.compose.service": "web", "secret": "do-not-send"},
				"Ports": []map[string]any{
					{"IP": "::", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
					{"PrivatePort": 9000, "Type": "tcp"},
					{"IP": "0.0.0.0", "PrivatePort": 443, "PublicPort": 8443, "Type": "tcp"},
					{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
				}},
			{"Id": "c2", "Names": []string{"/linked/alias", "/db"}, "Image": "postgres:16", "ImageID": "sha256:pg", "State": "exited", "Status": "Exited (3) 1 day ago", "Created": 50},
			{"Id": "c3", "Names": []string{"/orphan"}, "Image": "sha256:gone", "ImageID": "sha256:gone", "State": "running", "Status": "Up 1 minute (health: starting)", "Created": 10},
			{"Id": "c4", "Names": []string{"/web-not-recreated"}, "Image": "sha256:old", "ImageID": "sha256:old", "State": "running", "Status": "Up 3 days", "Created": 5},
		})

	case version + "/containers/c1/json":
		inspect(proxyHost+"/web:current", "2026-11-01T10:00:00.123456789Z", "0001-01-01T00:00:00Z", 0)

	case version + "/containers/c2/json":
		inspect("postgres:16", "2026-10-30T08:00:00Z", "2026-10-31T09:30:00Z", 3)

	case version + "/containers/c3/json":
		inspect("sha256:gone", "2026-11-01T09:59:00Z", "0001-01-01T00:00:00Z", 0)

	case version + "/containers/c4/json":
		inspect(proxyHost+"/web:current", "2026-10-29T12:00:00Z", "0001-01-01T00:00:00Z", 0)

	case version + "/images/json":
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"Id": "sha256:old", "RepoDigests": []string{proxyHost + "/web@sha256:aaa"}},
			{"Id": "sha256:new", "RepoDigests": []string{proxyHost + "/web@sha256:bbb"}},
			{"Id": "sha256:pg", "RepoDigests": []string{"postgres@sha256:ccc"}},
			{"Id": "sha256:unused", "RepoDigests": []string{"busybox@sha256:ddd"}},
		})

	case version + "/images/" + proxyHost + "/web:current/json":
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "sha256:new"})

	case version + "/images/postgres:16/json":
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "No such image"})

	case version + "/events":
		if eventsBroken {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.(http.Flusher).Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-d.events:
				_, _ = w.Write([]byte(event + "\n"))
				w.(http.Flusher).Flush()
			}
		}

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *fakeDocker) set(change func(*fakeDocker)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	change(d)
}

func (d *fakeDocker) takePaths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	paths := d.paths
	d.paths = nil

	return paths
}

func (d *fakeDocker) event(kind, action, id string) {
	encoded, _ := json.Marshal(map[string]any{"Type": kind, "Action": action, "Actor": map[string]string{"ID": id}})
	d.events <- string(encoded)
}

type testAgent struct {
	agent    *Agent
	address  string
	dataPath string
	stop     func()
}

func startAgent(t *testing.T, docker *fakeDocker, proxyOnly bool, dataPath string) *testAgent {
	t.Helper()

	if dataPath == "" {
		dataPath = t.TempDir()
	}

	a, err := New(Config{DataPath: dataPath, DockerHost: "unix://" + docker.socket, ProxyOnly: proxyOnly}, "v1.9.0")

	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		if err := a.Serve(ctx, listener); err != nil {
			t.Errorf("serve: %v", err)
		}

		close(done)
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}

	t.Cleanup(stop)

	return &testAgent{agent: a, address: listener.Addr().String(), dataPath: dataPath, stop: stop}
}

func (ta *testAgent) waitWatching(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for !ta.agent.cache.isWatching() {
		if time.Now().After(deadline) {
			t.Fatal("the agent never started watching Docker events")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func newClient(t *testing.T) *agentclient.Client {
	t.Helper()

	data, err := agentapi.NewIdentity("dockvmap")

	if err != nil {
		t.Fatal(err)
	}

	identity, err := agentapi.ParseIdentity(data)

	if err != nil {
		t.Fatal(err)
	}

	return agentclient.New(identity, "v1.9.0")
}

func adopt(t *testing.T, client *agentclient.Client, ta *testAgent) string {
	t.Helper()

	adoption, seen, err := client.Adopt(context.Background(), ta.address, "")

	if err != nil {
		t.Fatalf("adopt: %v", err)
	}

	if seen != ta.agent.Fingerprint() || adoption.ProtocolVersion != agentapi.ProtocolVersion || adoption.AgentVersion != "v1.9.0" {
		t.Fatalf("adoption = %+v, seen = %s, want %s", adoption, seen, ta.agent.Fingerprint())
	}

	return seen
}

func snapshot(t *testing.T, client *agentclient.Client, ta *testAgent, pinned, fingerprint string) *agentapi.Report {
	t.Helper()

	report, err := client.Snapshot(context.Background(), ta.address, pinned, fingerprint, proxyHost)

	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	return report
}

func TestAgentReportsTheHostSnapshot(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)

	report := snapshot(t, client, ta, adopt(t, client, ta), "")

	if report.ProtocolVersion != agentapi.ProtocolVersion || report.AgentVersion != "v1.9.0" || report.DockerError != "" || len(report.Fingerprint) != 64 {
		t.Fatalf("report header = %+v", report)
	}

	if report.Host.Hostname != "srv2" || report.Host.DockerVersion != "29.0.1" || report.Host.CPUs != 4 {
		t.Fatalf("host = %+v", report.Host)
	}

	byName := map[string]agentapi.Container{}

	for _, c := range report.Containers {
		byName[c.Name] = c
	}

	web, db, orphan, notRecreated := byName["web-1"], byName["db"], byName["orphan"], byName["web-not-recreated"]

	if len(byName) != 4 {
		t.Fatalf("containers = %+v", report.Containers)
	}

	wantPorts := []agentapi.Port{
		{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: "::", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: "0.0.0.0", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
	}

	if !slices.Equal(web.Ports, wantPorts) || db.Ports != nil {
		t.Errorf("ports = %+v, want only published ones in a stable order", web.Ports)
	}

	if web.Health != "healthy" || web.ComposeProject != "shop" || web.ComposeService != "web" || web.RefImageID != "sha256:new" || web.StartedAt != 1_793_527_200 || web.FinishedAt != 0 {
		t.Errorf("web = %+v", web)
	}

	if db.RefImageID != "" || db.State != "exited" || db.ExitCode != 3 || db.FinishedAt != 1_793_439_000 {
		t.Errorf("db = %+v", db)
	}

	if orphan.Health != "starting" || orphan.Image != "sha256:gone" || orphan.StartedAt != 1_793_527_140 {
		t.Errorf("orphan = %+v", orphan)
	}

	if notRecreated.Image != proxyHost+"/web:current" || notRecreated.ImageID != "sha256:old" || notRecreated.RefImageID != "sha256:new" {
		t.Errorf("a container whose tag moved on must keep its image name: %+v", notRecreated)
	}

	ids := []string{}

	for _, image := range report.Images {
		ids = append(ids, image.ID)
	}

	if strings.Join(ids, ",") != "sha256:new,sha256:old,sha256:pg" {
		t.Errorf("images = %v, want only the ones containers use, sorted", ids)
	}

	encoded, _ := json.Marshal(report)

	if strings.Contains(string(encoded), "do-not-send") || strings.Contains(string(encoded), "Up 2 hours") {
		t.Error("the report leaked a label or Docker's time-dependent status text")
	}

	docker.mu.Lock()
	defer docker.mu.Unlock()

	for i, method := range docker.methods {
		if method != http.MethodGet {
			t.Errorf("request %s used %s, the agent must only read", docker.paths[i], method)
		}
	}
}

func TestAgentAnswersNotModifiedUntilSomethingChanges(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	ta.waitWatching(t)

	first := snapshot(t, client, ta, pinned, "")
	docker.takePaths()

	if report := snapshot(t, client, ta, pinned, first.Fingerprint); report != nil {
		t.Fatalf("an unchanged host should answer not modified, got %+v", report)
	}

	if paths := docker.takePaths(); !slices.Equal(paths, []string{"/v1.41/containers/json"}) {
		t.Fatalf("docker requests = %v, want only the container list while events keep the cache valid", paths)
	}

	docker.set(func(d *fakeDocker) { d.webState = "exited" })
	docker.event("container", "die", "c1")

	deadline := time.Now().Add(5 * time.Second)

	for {
		report := snapshot(t, client, ta, pinned, first.Fingerprint)

		if report != nil {
			if report.Fingerprint == first.Fingerprint || !slices.ContainsFunc(report.Containers, func(c agentapi.Container) bool { return c.ID == "c1" && c.State == "exited" }) {
				t.Fatalf("changed report = %+v", report)
			}

			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the change never showed up")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

func TestAgentIgnoresHealthcheckExecEvents(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	ta.waitWatching(t)
	snapshot(t, client, ta, pinned, "")

	for range 3 {
		docker.event("container", "exec_create: /bin/sh -c true", "c1")
		docker.event("container", "exec_die", "c1")
	}

	time.Sleep(200 * time.Millisecond)
	docker.takePaths()
	snapshot(t, client, ta, pinned, "")

	if paths := docker.takePaths(); !slices.Equal(paths, []string{"/v1.41/containers/json"}) {
		t.Fatalf("docker requests = %v, exec events must not re-read the container", paths)
	}
}

func TestAgentRereadsImagesAfterImageEvents(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	ta.waitWatching(t)
	snapshot(t, client, ta, pinned, "")

	docker.event("image", "pull", proxyHost+"/web:current")

	deadline := time.Now().Add(5 * time.Second)

	for {
		docker.takePaths()
		snapshot(t, client, ta, pinned, "")
		paths := docker.takePaths()

		if slices.Contains(paths, "/v1.41/images/json") {
			if !slices.Contains(paths, "/v1.41/images/"+proxyHost+"/web:current/json") {
				t.Fatalf("docker requests = %v, want the tag looked up again", paths)
			}

			if slices.Contains(paths, "/v1.41/containers/c1/json") || slices.Contains(paths, "/v1.41/info") {
				t.Fatalf("docker requests = %v, an image event must not re-read containers or info", paths)
			}

			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("images were never re-read, last requests = %v", paths)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

func TestAgentReadsDockerInFullWhileEventsAreDown(t *testing.T) {
	docker := newFakeDocker(t)
	docker.set(func(d *fakeDocker) { d.eventsBroken = true })
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	snapshot(t, client, ta, pinned, "")
	docker.takePaths()
	snapshot(t, client, ta, pinned, "")

	if paths := docker.takePaths(); !slices.Contains(paths, "/v1.41/containers/c1/json") || !slices.Contains(paths, "/v1.41/images/json") {
		t.Fatalf("docker requests = %v, without events every poll must read Docker in full", paths)
	}
}

func TestAgentProxyOnlyReportsProxyContainers(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, true, "")
	client := newClient(t)

	report := snapshot(t, client, ta, adopt(t, client, ta), "")

	names := []string{}

	for _, c := range report.Containers {
		names = append(names, c.Name)
	}

	slices.Sort(names)

	if strings.Join(names, ",") != "web-1,web-not-recreated" || len(report.Images) != 2 {
		t.Fatalf("containers = %v, images = %+v", names, report.Images)
	}
}

func TestAgentReportsDockerErrors(t *testing.T) {
	docker := newFakeDocker(t)
	docker.set(func(d *fakeDocker) { d.broken = true })
	ta := startAgent(t, docker, false, "")
	client := newClient(t)

	report := snapshot(t, client, ta, adopt(t, client, ta), "")

	if !strings.Contains(report.DockerError, "daemon is sad") || len(report.Containers) != 0 || report.Fingerprint == "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestAgentReportsAHungDockerBeforeDockVMapGivesUp(t *testing.T) {
	docker := newFakeDocker(t)
	docker.set(func(d *fakeDocker) { d.hung = true })
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	report, err := client.Snapshot(ctx, ta.address, pinned, "", proxyHost)

	if err != nil || !strings.Contains(report.DockerError, "didn't answer within 5s") {
		t.Fatalf("report = %+v, err = %v; a hung Docker must come back as a Docker error inside DockVMap's poll timeout", report, err)
	}
}

func TestAgentNegotiatesAHigherMinimumAPIVersion(t *testing.T) {
	docker := newFakeDocker(t)
	docker.minAPI = "1.44"
	ta := startAgent(t, docker, false, "")
	client := newClient(t)

	if report := snapshot(t, client, ta, adopt(t, client, ta), ""); report.DockerError != "" || len(report.Containers) != 4 {
		t.Fatalf("report = %+v", report)
	}
}

func TestAgentWaitsToBeAdopted(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)

	if _, err := client.Snapshot(context.Background(), ta.address, ta.agent.Fingerprint(), "", proxyHost); !errors.Is(err, agentclient.ErrNotAdopted) {
		t.Fatalf("snapshot before adoption = %v, want ErrNotAdopted", err)
	}

	pinned := adopt(t, client, ta)

	if _, seen, err := client.Adopt(context.Background(), ta.address, pinned); err != nil || seen != pinned {
		t.Fatalf("adopting again from the same DockVMap = %s, %v", seen, err)
	}
}

func TestAgentPairsWithTheFirstDockVMapOnly(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	first, other := newClient(t), newClient(t)
	pinned := adopt(t, first, ta)

	if _, _, err := other.Adopt(context.Background(), ta.address, ""); !errors.Is(err, agentclient.ErrPairedElsewhere) {
		t.Fatalf("adopt by another DockVMap = %v, want ErrPairedElsewhere", err)
	}

	if _, err := other.Snapshot(context.Background(), ta.address, pinned, "", proxyHost); !errors.Is(err, agentclient.ErrPairedElsewhere) {
		t.Fatalf("snapshot by another DockVMap = %v, want ErrPairedElsewhere", err)
	}

	snapshot(t, first, ta, pinned, "")
}

func TestAgentPairsOnceWhenTwoDockVMapsRace(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	results := make(chan error, 2)

	for _, client := range []*agentclient.Client{newClient(t), newClient(t)} {
		go func() {
			_, _, err := client.Adopt(context.Background(), ta.address, "")
			results <- err
		}()
	}

	first, second := <-results, <-results

	if (first == nil) == (second == nil) || !errors.Is(errors.Join(first, second), agentclient.ErrPairedElsewhere) {
		t.Fatalf("results = %v, %v; want exactly one adoption and one refusal", first, second)
	}
}

func TestAgentKeepsItsIdentityAndPairingAcrossRestarts(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")
	client := newClient(t)
	pinned := adopt(t, client, ta)

	ta.stop()

	restarted := startAgent(t, docker, false, ta.dataPath)

	if restarted.agent.Fingerprint() != pinned {
		t.Fatalf("fingerprint after restart = %s, want %s", restarted.agent.Fingerprint(), pinned)
	}

	snapshot(t, client, restarted, pinned, "")

	restarted.stop()

	replaced := startAgent(t, docker, false, "")

	if _, err := client.Snapshot(context.Background(), replaced.address, pinned, "", proxyHost); !errors.Is(err, agentclient.ErrIdentityChanged) {
		t.Fatalf("snapshot from an agent with a new identity = %v, want ErrIdentityChanged", err)
	}
}

func TestAgentRefusesClientsWithoutACertificate(t *testing.T) {
	docker := newFakeDocker(t)
	ta := startAgent(t, docker, false, "")

	plain := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

	resp, err := plain.Get("https://" + ta.address + agentapi.SnapshotPath)

	if err == nil {
		resp.Body.Close()
		t.Fatalf("a client without a certificate got %s", resp.Status)
	}
}

func TestErrorsExplainWhatToDo(t *testing.T) {
	silent, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	defer silent.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if _, _, err := newClient(t).Adopt(ctx, silent.Addr().String(), ""); err == nil || !strings.Contains(err.Error(), "didn't answer in time") {
		t.Errorf("an agent that never answers = %v", err)
	}

	missing, _ := newDockerClient("unix://" + filepath.Join(t.TempDir(), "missing.sock"))

	if _, err := missing.info(context.Background()); err == nil || !strings.Contains(err.Error(), "is the Docker socket mounted") {
		t.Errorf("missing socket = %v", err)
	}

	refused, _ := newDockerClient("tcp://127.0.0.1:1")

	if _, err := refused.info(context.Background()); err == nil || !strings.Contains(err.Error(), "check DOCKER_HOST") {
		t.Errorf("refused = %v", err)
	}

	if _, err := New(Config{DataPath: filepath.Join(t.TempDir(), "missing"), DockerHost: defaultDockerHost}, "v1"); err == nil || !strings.Contains(err.Error(), "saving the agent identity in") {
		t.Errorf("missing data path = %v", err)
	}
}

func TestHealthNamesAndTimes(t *testing.T) {
	if healthFromStatus("Up 5 minutes") != "" || healthFromStatus("Up 5 minutes (unhealthy)") != "unhealthy" {
		t.Error("health parsing")
	}

	id := strings.Repeat("a", 64)

	if containerName(dockerContainer{Names: []string{"/a/b", "/c"}}) != "c" ||
		containerName(dockerContainer{Names: []string{"/x/y"}}) != "x/y" ||
		containerName(dockerContainer{ID: id}) != id[:12] {
		t.Error("container names")
	}

	if unixSeconds("0001-01-01T00:00:00Z") != 0 || unixSeconds("garbage") != 0 || unixSeconds("2026-11-01T10:00:00Z") != 1_793_527_200 {
		t.Error("docker times")
	}
}
