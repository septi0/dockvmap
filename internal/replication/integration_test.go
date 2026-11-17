//go:build integration

package replication_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	registryImage = "registry:3.0.0"
	indexType     = "application/vnd.oci.image.index.v1+json"
	manifestType  = "application/vnd.oci.image.manifest.v1+json"
	settle        = 45 * time.Second
)

var accept = []string{indexType, manifestType, "application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.docker.distribution.manifest.v2+json"}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()

	out, err := exec.Command(name, args...).CombinedOutput()

	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}

	return strings.TrimSpace(string(out))
}

func freeAddress(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}

	defer l.Close()

	return l.Addr().String()
}

func eventually(t *testing.T, what string, check func() (bool, string)) {
	t.Helper()

	deadline := time.Now().Add(settle)
	last := ""

	for time.Now().Before(deadline) {
		ok, detail := check()

		if ok {
			return
		}

		last = detail
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("%s didn't happen within %v; last: %s", what, settle, last)
}

func startRegistry(t *testing.T, name string) string {
	t.Helper()

	run(t, "docker", "run", "-d", "--name", name, "-p", "127.0.0.1::5000", "-e", "REGISTRY_STORAGE_DELETE_ENABLED=true", registryImage)
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })

	address := run(t, "docker", "port", name, "5000/tcp")

	eventually(t, name+" answering", func() (bool, string) {
		resp, err := http.Get("http://" + address + "/v2/")

		if err != nil {
			return false, err.Error()
		}

		resp.Body.Close()

		return resp.StatusCode == http.StatusOK, resp.Status
	})

	return address
}

type node struct {
	name    string
	binary  string
	dir     string
	env     map[string]string
	web     string
	proxy   string
	cmd     *exec.Cmd
	client  *http.Client
	logFile *os.File
}

func newNode(t *testing.T, binary, name string, env map[string]string) *node {
	t.Helper()

	n := &node{name: name, binary: binary, dir: t.TempDir(), env: env, web: freeAddress(t), proxy: freeAddress(t)}
	n.env["DOCKVMAP_WEB_SERVER_HTTP_LISTEN"] = n.web
	n.env["DOCKVMAP_PROXY_SERVER_LISTEN"] = n.proxy

	jar, _ := cookiejar.New(nil)
	n.client = &http.Client{Jar: jar, Timeout: 30 * time.Second}

	t.Cleanup(n.stop)

	return n
}

func (n *node) environ() []string {
	env := os.Environ()

	for key, value := range n.env {
		env = append(env, key+"="+value)
	}

	return env
}

func (n *node) start(t *testing.T) {
	t.Helper()

	logFile, err := os.OpenFile(filepath.Join(n.dir, "dockvmap.out"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)

	if err != nil {
		t.Fatal(err)
	}

	n.logFile = logFile
	n.cmd = exec.Command(n.binary, "-data-path", n.dir)
	n.cmd.Env = n.environ()
	n.cmd.Stdout, n.cmd.Stderr = logFile, logFile

	if err := n.cmd.Start(); err != nil {
		t.Fatalf("starting %s: %v", n.name, err)
	}

	eventually(t, n.name+" starting", func() (bool, string) {
		resp, err := http.Get("http://" + n.web + "/health")

		if err != nil {
			return false, err.Error()
		}

		resp.Body.Close()

		return resp.StatusCode == http.StatusOK, resp.Status
	})
}

func (n *node) stop() {
	if n.cmd == nil || n.cmd.Process == nil {
		return
	}

	n.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		n.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		n.cmd.Process.Kill()
		<-done
	}

	n.cmd = nil
	n.logFile.Close()
}

func (n *node) logs() string {
	data, _ := os.ReadFile(filepath.Join(n.dir, "dockvmap.out"))
	return string(data)
}

func (n *node) api(t *testing.T, method, path string, body any, out any) int {
	t.Helper()

	var reader io.Reader

	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}

	req, _ := http.NewRequest(method, "http://"+n.web+path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DockVMap", "1")

	resp, err := n.client.Do(req)

	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}

	defer resp.Body.Close()

	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}

	return resp.StatusCode
}

func (n *node) mustAPI(t *testing.T, method, path string, body any, out any) {
	t.Helper()

	if code := n.api(t, method, path, body, out); code >= 300 {
		t.Fatalf("%s %s on %s answered %d", method, path, n.name, code)
	}
}

func (n *node) ready(t *testing.T) (int, map[string]any) {
	t.Helper()

	resp, err := http.Get("http://" + n.web + "/health/ready")

	if err != nil {
		return 0, map[string]any{"error": err.Error()}
	}

	defer resp.Body.Close()

	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)

	return resp.StatusCode, body
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// pushImage pushes a synthetic amd64 + arm64 index the way a client would, and returns the index digest
func pushImage(t *testing.T, registry, repository, tag, seed string) string {
	t.Helper()

	blob := func(content string) string {
		data := []byte(content)
		digest := digestOf(data)

		resp, err := http.Post("http://"+registry+"/v2/"+repository+"/blobs/uploads/", "", nil)

		if err != nil || resp.StatusCode != http.StatusAccepted {
			t.Fatalf("starting an upload to %s: %v %v", registry, err, resp)
		}

		resp.Body.Close()

		location := resp.Header.Get("Location")

		if strings.HasPrefix(location, "/") {
			location = "http://" + registry + location
		}

		separator := "?"

		if strings.Contains(location, "?") {
			separator = "&"
		}

		req, _ := http.NewRequest(http.MethodPut, location+separator+"digest="+digest, bytes.NewReader(data))
		resp, err = http.DefaultClient.Do(req)

		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("uploading a blob to %s: %v %v", registry, err, resp)
		}

		resp.Body.Close()

		return digest
	}

	manifest := func(reference, mediaType string, content any) string {
		data, _ := json.Marshal(content)
		req, _ := http.NewRequest(http.MethodPut, "http://"+registry+"/v2/"+repository+"/manifests/"+reference, bytes.NewReader(data))
		req.Header.Set("Content-Type", mediaType)
		resp, err := http.DefaultClient.Do(req)

		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("pushing %s:%s to %s: %v %v", repository, reference, registry, err, resp)
		}

		resp.Body.Close()

		return digestOf(data)
	}

	platform := func(arch string) map[string]any {
		data, _ := json.Marshal(map[string]any{
			"schemaVersion": 2,
			"mediaType":     manifestType,
			"config":        map[string]any{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": blob(`{"architecture":"` + arch + `","os":"linux","seed":"` + seed + `"}`), "size": 1},
			"layers":        []map[string]any{{"mediaType": "application/vnd.oci.image.layer.v1.tar", "digest": blob("base layer"), "size": 1}, {"mediaType": "application/vnd.oci.image.layer.v1.tar", "digest": blob(arch + " layer " + seed), "size": 1}},
		})

		var content map[string]any
		json.Unmarshal(data, &content)
		digest := manifest(digestOf(data), manifestType, content)

		return map[string]any{"mediaType": manifestType, "digest": digest, "size": len(data), "platform": map[string]string{"architecture": arch, "os": "linux"}}
	}

	return manifest(tag, indexType, map[string]any{
		"schemaVersion": 2,
		"mediaType":     indexType,
		"manifests":     []map[string]any{platform("amd64"), platform("arm64")},
	})
}

func manifestDigest(registry, repository, reference string) (string, int) {
	req, _ := http.NewRequest(http.MethodHead, "http://"+registry+"/v2/"+repository+"/manifests/"+reference, nil)
	req.Header["Accept"] = accept
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return err.Error(), 0
	}

	resp.Body.Close()

	return resp.Header.Get("Docker-Content-Digest"), resp.StatusCode
}

func TestReplicationIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker isn't available")
	}

	prefix := fmt.Sprintf("dvm-testlab-%d", os.Getpid())
	binary := filepath.Join(t.TempDir(), "dockvmap")
	run(t, "go", "build", "-o", binary, "github.com/septi0/dockvmap/cmd/dockvmap")

	primaryRegistry := startRegistry(t, prefix+"-primary-registry")
	followerRegistry := startRegistry(t, prefix+"-follower-registry")
	upstream := startRegistry(t, prefix+"-upstream")
	replicationAddress := freeAddress(t)

	primary := newNode(t, binary, "primary", map[string]string{
		"DOCKVMAP_INTERNAL_REGISTRY_ENABLED": "true",
		"DOCKVMAP_INTERNAL_REGISTRY_ADDRESS": primaryRegistry,
		"DOCKVMAP_REPLICATION_ENABLED":       "true",
		"DOCKVMAP_REPLICATION_LISTEN":        replicationAddress,
	})
	primary.start(t)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("primary log:\n%s", primary.logs())
		}
	})

	primary.mustAPI(t, http.MethodPost, "/api/setup", map[string]string{"username": "admin", "email": "admin@example.com", "password": "testlab-password"}, nil)
	primary.mustAPI(t, http.MethodPost, "/api/login", map[string]string{"username": "admin", "password": "testlab-password"}, nil)

	var token struct{ Token string }
	primary.mustAPI(t, http.MethodPost, "/api/replication/tokens", map[string]string{"label": "standby-1"}, &token)

	var upstreamRegistry struct{ ID int64 }
	primary.mustAPI(t, http.MethodPost, "/api/registries", map[string]any{"registry": upstream, "options": map[string]bool{"insecure": true}}, &upstreamRegistry)
	alpineV1 := pushImage(t, upstream, "library/alpine", "3.20", "upstream")
	primary.mustAPI(t, http.MethodPost, "/api/images", map[string]any{"name": "alpine", "registryId": upstreamRegistry.ID, "repository": "library/alpine", "tag": "3.20"}, nil)

	webV1 := pushImage(t, primary.proxy, "team/web", "v1", "v1")
	webV2 := pushImage(t, primary.proxy, "team/web", "v2", "v2")

	var registries []struct {
		ID       int64
		Internal bool
	}
	primary.mustAPI(t, http.MethodGet, "/api/registries", nil, &registries)

	internalID := int64(0)

	for _, r := range registries {
		if r.Internal {
			internalID = r.ID
		}
	}

	primary.mustAPI(t, http.MethodPost, "/api/images", map[string]any{"name": "web", "registryId": internalID, "repository": "team/web", "tag": "v1"}, nil)

	follower := newNode(t, binary, "follower", map[string]string{
		"DOCKVMAP_INTERNAL_REGISTRY_ENABLED": "true",
		"DOCKVMAP_INTERNAL_REGISTRY_ADDRESS": followerRegistry,
		"DOCKVMAP_FOLLOWER_ENABLED":          "true",
		"DOCKVMAP_FOLLOWER_PRIMARY":          replicationAddress,
		"DOCKVMAP_FOLLOWER_TOKEN":            token.Token,
	})

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("follower log:\n%s", follower.logs())
		}
	})

	pullDigest := func(n *node, name string) (string, int) {
		return manifestDigest(n.proxy, name, "current")
	}

	t.Run("a multi-arch push reaches the follower", func(t *testing.T) {
		follower.start(t)

		eventually(t, "the follower being ready", func() (bool, string) {
			code, body := follower.ready(t)
			return code == http.StatusOK, fmt.Sprint(body)
		})

		for tag, want := range map[string]string{"v1": webV1, "v2": webV2} {
			if got, _ := manifestDigest(followerRegistry, "team/web", tag); got != want {
				t.Errorf("follower registry team/web:%s = %q, want %q", tag, got, want)
			}
		}

		if got, code := pullDigest(follower, "web"); got != webV1 {
			t.Errorf("pull of web:current through the follower = %q (%d), want %q", got, code, webV1)
		}

		started := time.Now()
		apiV1 := pushImage(t, primary.proxy, "team/api", "v1", "api")

		eventually(t, "team/api:v1 on the follower", func() (bool, string) {
			got, _ := manifestDigest(followerRegistry, "team/api", "v1")
			return got == apiV1, got
		})

		t.Logf("a push reached the follower's registry in %v", time.Since(started).Round(time.Millisecond))

		webV3 := pushImage(t, primary.proxy, "team/web", "v3", "v3")

		eventually(t, "a new tag in a repository the follower already has", func() (bool, string) {
			got, _ := manifestDigest(followerRegistry, "team/web", "v3")
			return got == webV3, got
		})
	})

	t.Run("a virtual tag moves to a tag right after its push", func(t *testing.T) {
		pushImage(t, primary.proxy, "team/deploy", "v1", "deploy-v1")
		primary.mustAPI(t, http.MethodPost, "/api/images", map[string]any{"name": "deploy", "registryId": internalID, "repository": "team/deploy", "tag": "v1"}, nil)

		var images struct {
			Images []struct{ ID int64 }
		}
		primary.mustAPI(t, http.MethodGet, "/api/images?search=deploy", nil, &images)

		deployV2 := pushImage(t, primary.proxy, "team/deploy", "v2", "deploy-v2")

		if code := primary.api(t, http.MethodPut, fmt.Sprintf("/api/images/%d/tag", images.Images[0].ID), map[string]string{"tag": "v2"}, nil); code != http.StatusOK {
			t.Fatalf("moving deploy to v2 right after its push answered %d, want 200", code)
		}

		if got, code := pullDigest(primary, "deploy"); got != deployV2 {
			t.Errorf("pull of deploy:current = %q (%d), want the pushed v2 %q", got, code, deployV2)
		}
	})

	backup := filepath.Join(t.TempDir(), "primary-before.db")
	run(t, binary, "-data-path", primary.dir, "-backup", backup)

	t.Run("the follower's web port serves only health and a page naming the primary", func(t *testing.T) {
		for path, want := range map[string]int{"/": http.StatusOK, "/api/images": http.StatusNotFound, "/api/setup": http.StatusNotFound, "/health/ready": http.StatusOK} {
			resp, err := http.Get("http://" + follower.web + path)

			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != want {
				t.Errorf("GET %s on the follower = %d, want %d", path, resp.StatusCode, want)
			}

			if path != "/health/ready" && !strings.Contains(string(body), "follower of "+replicationAddress) {
				t.Errorf("GET %s on the follower = %q, want it to name the primary", path, body)
			}
		}
	})

	t.Run("a follower that was down catches up", func(t *testing.T) {
		follower.stop()

		var images struct {
			Images []struct{ ID int64 }
		}
		primary.mustAPI(t, http.MethodGet, "/api/images?search=web", nil, &images)
		primary.mustAPI(t, http.MethodPut, fmt.Sprintf("/api/images/%d/tag", images.Images[0].ID), map[string]string{"tag": "v2"}, nil)
		workerV1 := pushImage(t, primary.proxy, "team/worker", "v1", "worker")

		follower.start(t)

		eventually(t, "web:current moving to v2 on the follower", func() (bool, string) {
			got, _ := pullDigest(follower, "web")
			return got == webV2, got
		})

		eventually(t, "team/worker:v1 on the follower", func() (bool, string) {
			got, _ := manifestDigest(followerRegistry, "team/worker", "v1")
			return got == workerV1, got
		})
	})

	t.Run("a primary restored from an older backup stops the follower until a resync", func(t *testing.T) {
		primary.stop()

		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(filepath.Join(primary.dir, "dockvmap.db"+suffix))
		}

		data, err := os.ReadFile(backup)

		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(primary.dir, "dockvmap.db"), data, 0o600); err != nil {
			t.Fatal(err)
		}

		primary.start(t)

		eventually(t, "the follower stopping on the older history", func() (bool, string) {
			_, body := follower.ready(t)
			return body["sync"] == "diverged", fmt.Sprint(body)
		})

		if code, body := follower.ready(t); code != http.StatusOK {
			t.Errorf("a diverged follower answers %d %v, want ready: its data is still good", code, body)
		}

		if got, _ := pullDigest(follower, "web"); got != webV2 {
			t.Errorf("web:current on the stopped follower = %q, want it unchanged at v2", got)
		}

		resync := exec.Command(binary, "-data-path", follower.dir, "-resync", "-yes")
		resync.Env = follower.environ()

		if out, err := resync.CombinedOutput(); err != nil {
			t.Fatalf("-resync: %v\n%s", err, out)
		}

		eventually(t, "the follower taking the primary's data", func() (bool, string) {
			got, _ := pullDigest(follower, "web")
			_, body := follower.ready(t)
			return got == webV1 && body["sync"] == "running", fmt.Sprint(got, body)
		})

		primary.mustAPI(t, http.MethodPost, "/api/login", map[string]string{"username": "admin", "password": "testlab-password"}, nil)
	})

	t.Run("a delete through the primary is applied on the follower", func(t *testing.T) {
		code := 0
		req, _ := http.NewRequest(http.MethodDelete, "http://"+primary.proxy+"/v2/team/web/manifests/"+webV2, nil)

		if resp, err := http.DefaultClient.Do(req); err == nil {
			code = resp.StatusCode
			resp.Body.Close()
		}

		if code != http.StatusAccepted {
			t.Fatalf("deleting team/web@v2 through the primary answered %d, want 202", code)
		}

		eventually(t, "team/web:v2 gone from the follower", func() (bool, string) {
			_, status := manifestDigest(followerRegistry, "team/web", "v2")
			return status == http.StatusNotFound, fmt.Sprint(status)
		})

		if got, _ := manifestDigest(followerRegistry, "team/web", "v1"); got != webV1 {
			t.Errorf("team/web:v1 on the follower = %q after deleting v2, want it kept", got)
		}

		same := pushImage(t, upstream, "probe/tags", "a", "same")
		pushImage(t, upstream, "probe/tags", "b", "same")
		tagDelete, _ := http.NewRequest(http.MethodDelete, "http://"+upstream+"/v2/probe/tags/manifests/a", nil)
		resp, err := http.DefaultClient.Do(tagDelete)

		if err == nil {
			resp.Body.Close()
			b, _ := manifestDigest(upstream, "probe/tags", "b")
			t.Logf("registry:3 DELETE by tag answered %d; the other tag on the same digest is %s", resp.StatusCode, map[bool]string{true: "kept", false: "gone"}[b == same])
		}
	})

	t.Run("a setup that differs from the primary's isn't ready", func(t *testing.T) {
		follower.stop()
		follower.env["DOCKVMAP_VIRTUAL_TAG"] = "latest"
		follower.start(t)

		eventually(t, "the follower reporting the setup mismatch", func() (bool, string) {
			code, body := follower.ready(t)
			return code == http.StatusServiceUnavailable && body["sync"] == "setup-mismatch", fmt.Sprint(code, body)
		})

		follower.stop()
		delete(follower.env, "DOCKVMAP_VIRTUAL_TAG")
		follower.start(t)

		eventually(t, "the follower ready again", func() (bool, string) {
			code, body := follower.ready(t)
			return code == http.StatusOK, fmt.Sprint(body)
		})
	})

	t.Run("a follower outside replication.allowed_ips is refused", func(t *testing.T) {
		primary.stop()
		primary.env["DOCKVMAP_REPLICATION_ALLOWED_IPS"] = "10.255.255.1"
		primary.start(t)

		eventually(t, "the follower naming the refused address", func() (bool, string) {
			_, body := follower.ready(t)
			lastError, _ := body["lastError"].(string)
			return strings.Contains(lastError, "replication isn't allowed from 127.0.0.1; add it to replication.allowed_ips on the primary"), lastError
		})

		primary.stop()
		primary.env["DOCKVMAP_REPLICATION_ALLOWED_IPS"] = "10.255.255.1, 127.0.0.0/8"
		primary.start(t)

		eventually(t, "the follower syncing again from an allowed address", func() (bool, string) {
			_, body := follower.ready(t)
			return body["lastError"] == nil && body["sync"] == "running", fmt.Sprint(body)
		})
	})

	t.Run("with the primary down the follower serves pulls and refuses pushes", func(t *testing.T) {
		primary.stop()

		if got, code := pullDigest(follower, "web"); got != webV1 {
			t.Errorf("web:current through the follower = %q (%d), want %q", got, code, webV1)
		}

		if got, code := pullDigest(follower, "alpine"); got != alpineV1 {
			t.Errorf("alpine:current from the upstream through the follower = %q (%d), want %q", got, code, alpineV1)
		}

		if code, body := follower.ready(t); code != http.StatusOK {
			t.Errorf("follower with the primary down answers %d %v, want ready so failover works", code, body)
		}

		resp, err := http.Post("http://"+follower.proxy+"/v2/team/web/blobs/uploads/", "", nil)

		if err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("a push to the follower answered %v %v, want 405", resp, err)
		}

		if resp != nil {
			resp.Body.Close()
		}
	})
}
