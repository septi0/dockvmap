package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/service"
)

type fakeImages struct{ images map[string]*model.Image }

func (f fakeImages) Resolve(_ context.Context, name string) (*model.Image, error) {
	return f.images[name], nil
}

func (fakeImages) OnTagChanged(func(names ...string)) {}

type fakeTokens map[string]*model.ProxyToken

func (f fakeTokens) Verify(_ context.Context, token string) (*model.ProxyToken, error) {
	return f[token], nil
}

type registryCall struct {
	method, path, authorization string
}

type fakeRegistry struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []registryCall
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()

	f := &fakeRegistry{}
	f.server = httptest.NewServer(f.handler())
	t.Cleanup(f.server.Close)

	return f
}

func newFakeTLSRegistry(t *testing.T) *fakeRegistry {
	t.Helper()

	f := &fakeRegistry{}
	f.server = httptest.NewTLSServer(f.handler())
	t.Cleanup(f.server.Close)

	return f
}

func (f *fakeRegistry) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr := io.ReadAll(r.Body)

		f.mu.Lock()
		f.calls = append(f.calls, registryCall{r.Method, r.URL.RequestURI(), r.Header.Get("Authorization")})
		f.mu.Unlock()

		switch {
		case readErr != nil:
			w.WriteHeader(http.StatusBadRequest)

		case strings.HasPrefix(r.URL.Path, "/v2/locked/"):
			w.Header().Set("Www-Authenticate", `Basic realm="Registry Realm"`)
			w.WriteHeader(http.StatusUnauthorized)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/blobs/uploads/"):
			scheme := "http"

			if r.TLS != nil {
				scheme = "https"
			}

			w.Header().Set("Location", scheme+"://"+r.Host+r.URL.Path+"0f1e?_state=abc")
			w.WriteHeader(http.StatusAccepted)

		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/elsewhere/"):
			w.Header().Set("Location", "https://storage.example.com/blob")
			w.WriteHeader(http.StatusCreated)

		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/gone/"):
			w.WriteHeader(http.StatusAccepted)

		default:
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"from":"registry"}`)
		}
	})
}

func (f *fakeRegistry) address() string {
	return f.server.Listener.Addr().String()
}

func (f *fakeRegistry) recorded() []registryCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]registryCall(nil), f.calls...)
}

func newInternalProxy(internal string, auth bool, tokens fakeTokens) *Proxy {
	return newInternalProxyWith(internal, false, auth, tokens)
}

func newInternalProxyWith(internal string, useTLS, auth bool, tokens fakeTokens) *Proxy {
	cfg := &config.Config{VirtualTag: "current"}
	cfg.ProxyAuth.Enabled = auth

	return New(Deps{
		Config: cfg,
		Images: fakeImages{images: map[string]*model.Image{
			"web":   {ID: 1, Name: "web", Registry: internal, RegistryInternal: true, Repository: "team/web", Tag: "v1"},
			"myapp": {ID: 2, Name: "myapp", Registry: internal, RegistryInternal: true, Repository: "myapp", Tag: "v1"},
			"nginx": {ID: 3, Name: "nginx", Registry: "docker.io", Repository: "nginx", Tag: "1.27"},
		}},
		Tokens:              tokens,
		Usage:               &recordingUsage{},
		InternalRegistry:    internal,
		InternalRegistryTLS: useTLS,
	})
}

func serve(p *Proxy, method, path, token string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)

	if token != "" {
		req.SetBasicAuth("ci", token)
	}

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	return rec
}

func ociMessage(t *testing.T, rec *httptest.ResponseRecorder) (string, string) {
	t.Helper()

	var body ociError

	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Errors) == 0 {
		t.Fatalf("body %q isn't an OCI error: %v", rec.Body.String(), err)
	}

	return body.Errors[0].Code, body.Errors[0].Message
}

func TestFeatureOffAnswersAsBefore(t *testing.T) {
	p := newInternalProxy("", false, nil)

	if rec := serve(p, http.MethodPut, "/v2/team/web/manifests/v2", "", strings.NewReader("{}")); rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("PUT: status %d, Allow %q; want 405 with GET, HEAD", rec.Code, rec.Header().Get("Allow"))
	}

	if rec := serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown name: status %d, want 404", rec.Code)
	} else if code, _ := ociMessage(t, rec); code != "NAME_UNKNOWN" {
		t.Errorf("unknown name: code %q, want NAME_UNKNOWN", code)
	}

	for _, path := range []string{"/v2/web/tags/list", "/v2/_catalog"} {
		if rec := serve(p, http.MethodGet, path, "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}

	auth := newInternalProxy("", true, nil)

	if rec := serve(auth, http.MethodPost, "/v2/team/web/blobs/uploads/", "", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("write with auth on and no token: status %d, want the early 405", rec.Code)
	}
}

func TestPassthroughForwardsRealNames(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), true, fakeTokens{"pull": {ID: 1, Label: "host"}})

	rec := serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "pull", nil)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "registry") {
		t.Fatalf("status %d, body %q; want the registry's answer", rec.Code, rec.Body.String())
	}

	calls := registry.recorded()

	if len(calls) != 1 || calls[0].path != "/v2/team/api/manifests/v1" || calls[0].authorization != "" {
		t.Fatalf("registry calls = %+v, want one call without Authorization", calls)
	}

	if rec := serve(p, http.MethodGet, "/v2/_catalog?n=10", "pull", nil); rec.Code != http.StatusOK {
		t.Errorf("_catalog: status %d, want 200", rec.Code)
	}

	if rec := serve(p, http.MethodGet, "/v2/team/api/tags/list", "pull", nil); rec.Code != http.StatusOK {
		t.Errorf("tags/list: status %d, want 200", rec.Code)
	}
}

func TestPassthroughRewritesUploadLocations(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), false, nil)

	rec := serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "", nil)

	if rec.Code != http.StatusAccepted || rec.Header().Get("Location") != "/v2/team/api/blobs/uploads/0f1e?_state=abc" {
		t.Fatalf("status %d, Location %q; want 202 with a relative Location", rec.Code, rec.Header().Get("Location"))
	}

	rec = serve(p, http.MethodPut, "/v2/team/elsewhere/blobs/uploads/0f1e?digest=sha256:abc", "", strings.NewReader("layer"))

	if rec.Header().Get("Location") != "https://storage.example.com/blob" {
		t.Errorf("other Location = %q, want it untouched", rec.Header().Get("Location"))
	}
}

func TestPassthroughTurnsARegistryLoginIntoA502(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), false, nil)

	rec := serve(p, http.MethodGet, "/v2/locked/app/manifests/v1", "", nil)

	if rec.Code != http.StatusBadGateway || rec.Header().Get("Www-Authenticate") != "" {
		t.Fatalf("status %d, Www-Authenticate %q; want 502 without it", rec.Code, rec.Header().Get("Www-Authenticate"))
	}

	if code, message := ociMessage(t, rec); code != "UNAUTHORIZED" || !strings.Contains(message, "remove its htpasswd settings") {
		t.Errorf("error = %s %q, want the login message", code, message)
	}
}

func TestPassthroughReportsARegistryThatDoesntAnswer(t *testing.T) {
	p := newInternalProxy("127.0.0.1:1", false, nil)

	rec := serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "", nil)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", rec.Code)
	}

	if _, message := ociMessage(t, rec); !strings.Contains(message, "didn't answer; check that its container is running") {
		t.Errorf("message %q, want the no-answer message", message)
	}
}

func TestPassthroughUsesTLSWhenSet(t *testing.T) {
	registry := newFakeTLSRegistry(t)
	p := newInternalProxyWith(registry.address(), true, false, nil)

	rec := serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "", nil)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("untrusted certificate: status %d, want 502", rec.Code)
	}

	if _, message := ociMessage(t, rec); !strings.Contains(message, "has a certificate DockVMap doesn't accept") {
		t.Errorf("untrusted certificate: message %q, want the certificate message", message)
	}

	p.internal.proxy.Transport.(*http.Transport).TLSClientConfig = registry.server.Client().Transport.(*http.Transport).TLSClientConfig

	if rec := serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "registry") {
		t.Fatalf("status %d, body %q; want the registry's answer over TLS", rec.Code, rec.Body.String())
	}

	rec = serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "", nil)

	if rec.Code != http.StatusAccepted || rec.Header().Get("Location") != "/v2/team/api/blobs/uploads/0f1e?_state=abc" {
		t.Errorf("upload: status %d, Location %q; want 202 with a relative Location", rec.Code, rec.Header().Get("Location"))
	}
}

func TestPassthroughRefusesWritesToVirtualImages(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), false, nil)

	rec := serve(p, http.MethodPut, "/v2/web/manifests/v2", "", strings.NewReader("{}"))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}

	if code, message := ociMessage(t, rec); code != "UNSUPPORTED" || !strings.Contains(message, "web is a virtual image") || !strings.Contains(message, "like team/web") {
		t.Errorf("error = %s %q, want the virtual image message", code, message)
	}

	if calls := registry.recorded(); len(calls) != 0 {
		t.Errorf("registry calls = %+v, want none", calls)
	}
}

func TestPassthroughRefusesInvalidNames(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), false, nil)

	for _, path := range []string{"/v2/team/../etc/manifests/v1", "/v2/Team/App/manifests/v1", "/v2/team/app/unknown/x", "/v2/team/app"} {
		rec := serve(p, http.MethodGet, path, "", nil)

		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 400 or 404", path, rec.Code)
		}
	}

	if calls := registry.recorded(); len(calls) != 0 {
		t.Errorf("registry calls = %+v, want none", calls)
	}
}

func TestPassthroughNeedsCanPushForWrites(t *testing.T) {
	registry := newFakeRegistry(t)
	tokens := fakeTokens{"pull": {ID: 1, Label: "host"}, "push": {ID: 2, Label: "build", CanPush: true}}
	p := newInternalProxy(registry.address(), true, tokens)

	if rec := serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status %d, want 401", rec.Code)
	}

	rec := serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "pull", nil)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("pull-only token: status %d, want 403", rec.Code)
	}

	if code, message := ociMessage(t, rec); code != "DENIED" || !strings.Contains(message, "create a token with Can push") {
		t.Errorf("error = %s %q, want the Can push message", code, message)
	}

	if calls := registry.recorded(); len(calls) != 0 {
		t.Fatalf("registry calls = %+v, want none before a Can push token", calls)
	}

	if rec := serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "push", nil); rec.Code != http.StatusAccepted {
		t.Errorf("Can push token: status %d, want 202", rec.Code)
	}

	open := newInternalProxy(registry.address(), false, nil)

	if rec := serve(open, http.MethodPost, "/v2/team/api/blobs/uploads/", "", nil); rec.Code != http.StatusAccepted {
		t.Errorf("proxy_auth off: status %d, want 202", rec.Code)
	}
}

func TestUploadDeadlineMovesWhileDataArrives(t *testing.T) {
	previousRead, previousUpload, previousRefresh := readTimeout, uploadIdleTimeout, uploadDeadlineRefresh
	readTimeout, uploadIdleTimeout, uploadDeadlineRefresh = 150*time.Millisecond, 300*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() {
		readTimeout, uploadIdleTimeout, uploadDeadlineRefresh = previousRead, previousUpload, previousRefresh
	})

	registry := newFakeRegistry(t)
	server := httptest.NewServer(newInternalProxy(registry.address(), false, nil))
	defer server.Close()

	upload := func(pause time.Duration, chunks int) (int, error) {
		body, writer := io.Pipe()

		go func() {
			for range chunks {
				writer.Write([]byte(strings.Repeat("x", 1024)))
				time.Sleep(pause)
			}

			writer.Close()
		}()

		req, _ := http.NewRequest(http.MethodPatch, server.URL+"/v2/team/api/blobs/uploads/0f1e", body)
		resp, err := http.DefaultClient.Do(req)

		if err != nil {
			return 0, err
		}

		resp.Body.Close()

		return resp.StatusCode, nil
	}

	if status, err := upload(50*time.Millisecond, 12); err != nil || status != http.StatusOK {
		t.Errorf("steady upload over 600ms: status %d, err %v; want it to finish", status, err)
	}

	if status, err := upload(time.Second, 2); err == nil && status != http.StatusRequestTimeout {
		t.Errorf("stalled upload: status %d, want 408", status)
	}
}

func TestBlobCacheIsSkippedForTheInternalRegistry(t *testing.T) {
	cache := &blobcache.Cache{}
	p := &Proxy{cache: cache}

	if p.cacheFor(&model.Image{Registry: "registry:5000", RegistryInternal: true}) != nil {
		t.Error("a virtual image from the internal registry uses the blob cache")
	}

	if p.cacheFor(&model.Image{Registry: "docker.io"}) != cache {
		t.Error("a virtual image from another registry doesn't use the blob cache")
	}
}

func TestPushesToAVirtualImagesOwnRepositoryPassThrough(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), true, fakeTokens{"push": {ID: 2, Label: "build", CanPush: true}, "pull": {ID: 1, Label: "host"}})

	if rec := serve(p, http.MethodPost, "/v2/myapp/blobs/uploads/", "push", nil); rec.Code != http.StatusAccepted {
		t.Errorf("upload to myapp: status %d, want 202 from the registry", rec.Code)
	}

	if rec := serve(p, http.MethodPost, "/v2/myapp/blobs/uploads/", "pull", nil); rec.Code != http.StatusForbidden {
		t.Errorf("upload to myapp with a pull-only token: status %d, want 403", rec.Code)
	}

	rec := serve(p, http.MethodPut, "/v2/myapp/manifests/current", "push", strings.NewReader("{}"))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("push of the virtual tag: status %d, want 405", rec.Code)
	}

	if _, message := ociMessage(t, rec); !strings.Contains(message, "current is the virtual tag DockVMap manages for myapp") {
		t.Errorf("message %q, want the virtual tag message", message)
	}

	for _, name := range []string{"web", "nginx"} {
		if rec := serve(p, http.MethodPost, "/v2/"+name+"/blobs/uploads/", "push", nil); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("upload to %s: status %d, want 405", name, rec.Code)
		}
	}

	if calls := registry.recorded(); len(calls) != 1 {
		t.Errorf("registry calls = %+v, want only the upload to myapp", calls)
	}
}

func TestPassedThroughRequestsDontCountAsProxyRequests(t *testing.T) {
	registry := newFakeRegistry(t)
	p := newInternalProxy(registry.address(), false, nil)

	serve(p, http.MethodGet, "/v2/team/api/manifests/v1", "", nil)
	serve(p, http.MethodPost, "/v2/team/api/blobs/uploads/", "", nil)
	serve(p, http.MethodGet, "/v2/", "", nil)

	if got := p.metrics.Snapshot().TotalRequests; got != 1 {
		t.Errorf("total requests = %d, want 1 (only the request DockVMap answered itself)", got)
	}
}

type recordingChanges struct {
	mu      sync.Mutex
	changes []change
}

func (r *recordingChanges) Pushed(repository, tag, token string, info service.RequestInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, change{repository: repository, reference: tag, token: token, info: info})
}

func (r *recordingChanges) Deleted(repository, reference, token string, info service.RequestInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.changes = append(r.changes, change{repository: repository, reference: reference, token: token, deleted: true, info: info})
}

func TestTagPushesAreRecordedWithTheTokenAndClient(t *testing.T) {
	registry := newFakeRegistry(t)
	changes := &recordingChanges{}
	trusted, err := ipmatch.Parse([]string{"192.0.2.0/24"})

	if err != nil {
		t.Fatalf("parsing trusted proxies: %v", err)
	}

	cfg := &config.Config{}
	cfg.ProxyAuth.Enabled = true

	p := New(Deps{
		Config:           cfg,
		Images:           fakeImages{},
		Tokens:           fakeTokens{"push": {ID: 2, Label: "build", CanPush: true}},
		Usage:            &recordingUsage{},
		InternalRegistry: registry.address(),
		Changes:          changes,
		TrustedProxies:   trusted,
	})

	send := func(method, path string) int {
		req := httptest.NewRequest(method, path, strings.NewReader("{}"))
		req.SetBasicAuth("ci", "push")
		req.Header.Set("X-Forwarded-For", "10.20.30.40")
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, req)

		return rec.Code
	}

	digest := "sha256:" + strings.Repeat("a", 64)

	send(http.MethodPut, "/v2/team/elsewhere/manifests/v3")
	send(http.MethodPut, "/v2/team/elsewhere/manifests/"+digest)
	send(http.MethodPut, "/v2/team/api/manifests/v4")
	send(http.MethodDelete, "/v2/team/gone/manifests/"+digest)
	send(http.MethodDelete, "/v2/team/api/manifests/"+digest)

	changes.mu.Lock()
	defer changes.mu.Unlock()

	if len(changes.changes) != 2 {
		t.Fatalf("changes = %+v, want the tag push and the delete that the registry accepted", changes.changes)
	}

	if got := changes.changes[0]; got.repository != "team/elsewhere" || got.reference != "v3" || got.deleted || got.token != "build" || got.info.IP != "10.20.30.40" {
		t.Errorf("push = %+v, want team/elsewhere:v3 by build from 10.20.30.40", got)
	}

	if got := changes.changes[1]; got.repository != "team/gone" || got.reference != digest || !got.deleted || got.token != "build" {
		t.Errorf("delete = %+v, want team/gone@%s by build", got, digest)
	}
}

func TestUpstreamFailureMessages(t *testing.T) {
	dial := fmt.Errorf("requesting registry: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")})
	login := errors.New("fetching token: 401")

	cases := []struct {
		img  model.Image
		err  error
		want string
	}{
		{model.Image{Registry: "registry:5000", RegistryInternal: true}, dial, "the internal registry at registry:5000 didn't answer; check that its container is running"},
		{model.Image{Registry: "docker.io"}, dial, "the registry at docker.io didn't answer; try again later"},
		{model.Image{Registry: "docker.io"}, login, "the request to the registry at docker.io failed; check DockVMap's log"},
	}

	for _, tc := range cases {
		if got := upstreamFailureMessage(&tc.img, tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("%s, %v: message %q, want %q", tc.img.Registry, tc.err, got, tc.want)
		}
	}
}
