package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const proxyHost = "registry.test:5050"

type fakeDocker struct {
	server *httptest.Server
	socket string
	events chan string
	minAPI string

	mu       sync.Mutex
	paths    []string
	methods  []string
	broken   bool
	webState string
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
	broken, webState := d.broken, d.webState
	d.mu.Unlock()

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

type fakeController struct {
	server *httptest.Server

	mu          sync.Mutex
	reports     []agentapi.Report
	heartbeats  int
	token       string
	fingerprint string
	arrived     chan string
}

func newFakeController(t *testing.T) *fakeController {
	c := &fakeController{token: "secret", arrived: make(chan string, 20)}
	c.server = httptest.NewServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.server.Close)

	return c
}

func (c *fakeController) serve(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer "+c.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch r.URL.Path {
	case agentapi.ConfigPath:
		_ = json.NewEncoder(w).Encode(agentapi.Config{ProxyHost: proxyHost})

	case agentapi.ReportPath:
		var report agentapi.Report
		_ = json.NewDecoder(r.Body).Decode(&report)
		c.reports = append(c.reports, report)
		c.fingerprint = report.Fingerprint
		c.arrived <- "report"

	case agentapi.HeartbeatPath:
		var heartbeat agentapi.Heartbeat
		_ = json.NewDecoder(r.Body).Decode(&heartbeat)

		if heartbeat.Fingerprint != c.fingerprint {
			w.WriteHeader(agentapi.FullReportNeededStatus)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "full report needed"})
			c.arrived <- "conflict"

			return
		}

		c.heartbeats++
		c.arrived <- "heartbeat"
	}
}

func (c *fakeController) next(t *testing.T, within time.Duration) string {
	t.Helper()

	select {
	case kind := <-c.arrived:
		return kind
	case <-time.After(within):
		t.Fatalf("nothing arrived within %v", within)
		return ""
	}
}

func (c *fakeController) quiet(t *testing.T, during time.Duration) {
	t.Helper()

	select {
	case kind := <-c.arrived:
		t.Fatalf("unexpected %s", kind)
	case <-time.After(during):
	}
}

func (c *fakeController) lastReport() agentapi.Report {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.reports[len(c.reports)-1]
}

func startAgent(t *testing.T, docker *fakeDocker, controller *fakeController, proxyOnly bool) {
	t.Helper()

	base, _ := url.Parse(controller.server.URL)

	a, err := New(Config{URL: base, Token: "secret", DockerHost: "unix://" + docker.socket, ProxyOnly: proxyOnly}, "v1.9.0")

	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		a.Run(ctx)
		close(done)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func firstReport(t *testing.T, controller *fakeController) agentapi.Report {
	t.Helper()

	if kind := controller.next(t, 5*time.Second); kind != "report" {
		t.Fatalf("first message = %s, want a report", kind)
	}

	return controller.lastReport()
}

func warmUp(t *testing.T, docker *fakeDocker, controller *fakeController) {
	t.Helper()

	firstReport(t, controller)
	docker.event("container", "start", "c1")

	if kind := controller.next(t, eventDebounce+3*time.Second); kind != "heartbeat" {
		t.Fatalf("warm-up event should send a heartbeat, got %s", kind)
	}

	docker.takePaths()
}

func TestAgentReportsTheHostSnapshot(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	report := firstReport(t, controller)

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

func TestAgentSendsHeartbeatsWhenNothingChanged(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	warmUp(t, docker, controller)
	docker.event("container", "start", "c1")

	if kind := controller.next(t, eventDebounce+3*time.Second); kind != "heartbeat" {
		t.Fatalf("an event that changed nothing should send a heartbeat, got %s", kind)
	}

	if paths := docker.takePaths(); !slices.Equal(paths, []string{"/v1.41/containers/json", "/v1.41/containers/c1/json"}) {
		t.Fatalf("docker requests = %v, want the list and only the affected container", paths)
	}

	docker.set(func(d *fakeDocker) { d.webState = "exited" })
	docker.event("container", "die", "c1")

	if kind := controller.next(t, eventDebounce+3*time.Second); kind != "report" || controller.lastReport().Containers[0].State != "exited" {
		t.Fatalf("a real change should send a full report, got %s", kind)
	}
}

func TestAgentIgnoresHealthcheckExecEvents(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	firstReport(t, controller)

	for range 3 {
		docker.event("container", "exec_create: /bin/sh -c true", "c1")
		docker.event("container", "exec_die", "c1")
	}

	controller.quiet(t, eventDebounce+time.Second)
}

func TestAgentRereadsImagesAfterImageEvents(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	warmUp(t, docker, controller)
	docker.event("image", "pull", "registry.test:5050/web:current")
	controller.next(t, eventDebounce+3*time.Second)

	paths := docker.takePaths()

	if !slices.Contains(paths, "/v1.41/images/json") || !slices.Contains(paths, "/v1.41/images/"+proxyHost+"/web:current/json") {
		t.Fatalf("docker requests = %v, want images re-read", paths)
	}

	if slices.Contains(paths, "/v1.41/containers/c1/json") || slices.Contains(paths, "/v1.41/info") {
		t.Fatalf("docker requests = %v, an image event must not re-read containers or info", paths)
	}
}

func TestAgentSendsAFullReportWhenAsked(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	firstReport(t, controller)

	controller.mu.Lock()
	controller.fingerprint = ""
	controller.mu.Unlock()

	docker.event("container", "start", "c1")

	if kind := controller.next(t, eventDebounce+3*time.Second); kind != "conflict" {
		t.Fatalf("got %s, want the heartbeat to be refused", kind)
	}

	if kind := controller.next(t, time.Second); kind != "report" {
		t.Fatalf("got %s, want a full report right after", kind)
	}
}

func TestAgentProxyOnlyReportsProxyContainers(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	startAgent(t, docker, controller, true)

	report := firstReport(t, controller)

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
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	report := firstReport(t, controller)

	if !strings.Contains(report.DockerError, "daemon is sad") || len(report.Containers) != 0 || report.Fingerprint == "" {
		t.Fatalf("report = %+v", report)
	}
}

func TestAgentNegotiatesAHigherMinimumAPIVersion(t *testing.T) {
	docker := newFakeDocker(t)
	docker.minAPI = "1.44"
	controller := newFakeController(t)
	startAgent(t, docker, controller, false)

	if report := firstReport(t, controller); report.DockerError != "" || len(report.Containers) != 4 {
		t.Fatalf("report = %+v", report)
	}
}

func TestAgentRetriesAfterARejectedToken(t *testing.T) {
	docker := newFakeDocker(t)
	controller := newFakeController(t)
	controller.token = "regenerated"
	startAgent(t, docker, controller, false)

	time.Sleep(time.Second)

	controller.mu.Lock()
	controller.token = "secret"
	controller.mu.Unlock()

	if kind := controller.next(t, firstRetryDelay+3*time.Second); kind != "report" {
		t.Fatalf("got %s, want a report once the token works", kind)
	}
}

func TestErrorsExplainWhatToDo(t *testing.T) {
	unauthorized := (&controllerError{status: http.StatusUnauthorized}).Error()

	if !strings.Contains(unauthorized, "regenerate it on this host's page") {
		t.Errorf("401 message = %q", unauthorized)
	}

	base, _ := url.Parse("https://127.0.0.1:1")
	client := newControllerClient(Config{URL: base, Token: "x"}, "v1")

	if _, err := client.config(context.Background()); err == nil || !strings.Contains(err.Error(), "cannot reach dockvmap at https://127.0.0.1:1") {
		t.Errorf("unreachable = %v", err)
	}

	tlsServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsServer.Close()

	tlsBase, _ := url.Parse(tlsServer.URL)
	client = newControllerClient(Config{URL: tlsBase, Token: "x"}, "v1")

	if _, err := client.config(context.Background()); err == nil || !strings.Contains(err.Error(), "set DOCKVMAP_CA_FILE") {
		t.Errorf("untrusted certificate = %v", err)
	}

	missing, _ := newDockerClient("unix://" + filepath.Join(t.TempDir(), "missing.sock"))

	if _, err := missing.info(context.Background()); err == nil || !strings.Contains(err.Error(), "is the Docker socket mounted") {
		t.Errorf("missing socket = %v", err)
	}

	refused, _ := newDockerClient("tcp://127.0.0.1:1")

	if _, err := refused.info(context.Background()); err == nil || !strings.Contains(err.Error(), "check DOCKER_HOST") {
		t.Errorf("refused = %v", err)
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
