package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

type internalRegistryFixture struct {
	t     *testing.T
	ctx   context.Context
	store *store.Store
	audit *recordingAudit
}

func newInternalRegistryFixture(t *testing.T) *internalRegistryFixture {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { st.Close() })

	return &internalRegistryFixture{t: t, ctx: context.Background(), store: st, audit: &recordingAudit{}}
}

func (f *internalRegistryFixture) ensure(enabled bool, address string) error {
	return NewInternalRegistry(InternalRegistryDeps{
		Store:    f.store,
		Audit:    f.audit,
		Failures: stubFailures{},
		Enabled:  enabled,
		Address:  address,
	}).EnsureInternal(f.ctx)
}

func (f *internalRegistryFixture) internal() *model.RegistryInfo {
	f.t.Helper()

	info, err := f.store.GetInternalRegistry(f.ctx)

	if err != nil {
		f.t.Fatalf("getting the internal registry: %v", err)
	}

	return info
}

func (f *internalRegistryFixture) createRegistry(registry model.Registry) int64 {
	f.t.Helper()

	id, err := f.store.CreateRegistry(f.ctx, registry)

	if err != nil {
		f.t.Fatalf("creating registry: %v", err)
	}

	return id
}

func TestEnsureInternalCreatesTheRegistry(t *testing.T) {
	f := newInternalRegistryFixture(t)

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	info := f.internal()

	if info == nil || info.Registry != "registry:5000" || !info.Options.Insecure || info.AuthenticationConfigured {
		t.Fatalf("internal registry = %+v, want registry:5000, insecure, no login", info)
	}

	if f.audit.count(AuditTypeRegistryCreated) != 1 {
		t.Errorf("REGISTRY_CREATED audits = %d, want 1", f.audit.count(AuditTypeRegistryCreated))
	}

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("second EnsureInternal: %v", err)
	}

	if f.audit.count(AuditTypeRegistryCreated) != 1 || f.audit.count(AuditTypeRegistryUpdated) != 0 {
		t.Errorf("a second start with the same address changed something")
	}
}

func TestEnsureInternalTakesOverAnExistingEntry(t *testing.T) {
	f := newInternalRegistryFixture(t)

	id := f.createRegistry(model.Registry{Registry: "registry:5000"})

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	info := f.internal()

	if info == nil || info.ID != id || !info.Options.Insecure {
		t.Fatalf("internal registry = %+v, want the existing entry %d with Insecure on", info, id)
	}
}

func TestEnsureInternalMovesTheEntryAndItsImages(t *testing.T) {
	f := newInternalRegistryFixture(t)

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	id := f.internal().ID

	if err := f.store.CreateImage(f.ctx, &model.Image{Name: "web", RegistryID: id, Repository: "team/web", Tag: "v1"}); err != nil {
		t.Fatalf("creating image: %v", err)
	}

	if err := f.ensure(true, "images:5000"); err != nil {
		t.Fatalf("EnsureInternal with a new address: %v", err)
	}

	info := f.internal()

	if info == nil || info.ID != id || info.Registry != "images:5000" {
		t.Fatalf("internal registry = %+v, want entry %d at images:5000", info, id)
	}

	image, err := f.store.GetImage(f.ctx, "web")

	if err != nil || image == nil || image.Registry != "images:5000" {
		t.Fatalf("virtual image = %+v, %v; want it on images:5000", image, err)
	}
}

func TestEnsureInternalRefusesAnAddressUsedByAnotherEntry(t *testing.T) {
	f := newInternalRegistryFixture(t)

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	f.createRegistry(model.Registry{Registry: "images:5000"})

	err := f.ensure(true, "images:5000")

	if err == nil || !strings.Contains(err.Error(), "already used by another registry") {
		t.Fatalf("EnsureInternal error = %v, want the address conflict", err)
	}

	if info := f.internal(); info == nil || info.Registry != "registry:5000" {
		t.Fatalf("internal registry = %+v, want it unchanged", info)
	}
}

func TestEnsureInternalRefusesAnInvalidAddress(t *testing.T) {
	f := newInternalRegistryFixture(t)

	for _, address := range []string{"", "registry", "http://registry:5000", "registry:5000/v2", "registry:99999"} {
		if err := f.ensure(true, address); err == nil || !strings.Contains(err.Error(), "must be host:port") {
			t.Errorf("address %q: error = %v, want the host:port message", address, err)
		}
	}
}

func TestEnsureInternalClearsTheMarkWhenDisabled(t *testing.T) {
	f := newInternalRegistryFixture(t)

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	id := f.internal().ID

	if err := f.ensure(false, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal disabled: %v", err)
	}

	if info := f.internal(); info != nil {
		t.Fatalf("internal registry = %+v, want none", info)
	}

	info, err := f.store.GetRegistryInfoByID(f.ctx, id)

	if err != nil || info == nil || info.Registry != "registry:5000" {
		t.Fatalf("registry %d = %+v, %v; want it kept as a normal registry", id, info, err)
	}
}

func TestRegistriesRefuseChangesToTheInternalRegistry(t *testing.T) {
	f := newInternalRegistryFixture(t)

	if err := f.ensure(true, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	id := f.internal().ID
	registries := NewRegistries(RegistriesDeps{Store: f.store, Audit: f.audit})

	if _, err := registries.UpdateByID(f.ctx, model.RegistryUpdate{ID: id, Registry: "other:5000"}); !errors.Is(err, ErrRegistryManagedByConfig) {
		t.Errorf("UpdateByID error = %v, want ErrRegistryManagedByConfig", err)
	}

	if _, err := registries.DeleteByID(f.ctx, id); !errors.Is(err, ErrRegistryManagedByConfig) {
		t.Errorf("DeleteByID error = %v, want ErrRegistryManagedByConfig", err)
	}

	if err := f.ensure(false, "registry:5000"); err != nil {
		t.Fatalf("EnsureInternal disabled: %v", err)
	}

	if deleted, err := registries.DeleteByID(f.ctx, id); err != nil || !deleted {
		t.Errorf("DeleteByID after disabling = %v, %v; want it deleted", deleted, err)
	}
}

func TestInternalRegistryCheckRecordsAFailureOnceAndStopsWhenReachable(t *testing.T) {
	var mu sync.Mutex
	status := http.StatusServiceUnavailable
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		calls++
		w.WriteHeader(status)
	}))
	defer server.Close()

	failures := &recordingFailures{}
	registry := NewInternalRegistry(InternalRegistryDeps{Failures: failures, Enabled: true, Address: strings.TrimPrefix(server.URL, "http://")})
	registry.checkInterval = 10 * time.Millisecond

	done := make(chan struct{})

	go func() {
		registry.checkUntilReachable(context.Background())
		close(done)
	}()

	time.Sleep(60 * time.Millisecond)

	mu.Lock()
	status = http.StatusOK
	mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the check didn't stop after the registry answered")
	}

	failures.mu.Lock()
	got := append([]string(nil), failures.entries...)
	failures.mu.Unlock()

	if len(got) != 1 || !strings.Contains(got[0], "internal_registry ") || !strings.Contains(got[0], "instead of a registry") {
		t.Errorf("failures = %q, want one recorded failure", got)
	}

	mu.Lock()
	defer mu.Unlock()

	if calls < 3 {
		t.Errorf("calls = %d, want several retries", calls)
	}
}

func TestInternalRegistryCheckReportsALogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	registry := NewInternalRegistry(InternalRegistryDeps{Enabled: true, Address: strings.TrimPrefix(server.URL, "http://")})

	if err := registry.ping(context.Background()); !errors.Is(err, errInternalRegistryLogin) {
		t.Errorf("ping error = %v, want the login message", err)
	}
}

func TestInternalRegistryCheckReportsNoAnswer(t *testing.T) {
	registry := NewInternalRegistry(InternalRegistryDeps{Enabled: true, Address: "127.0.0.1:1"})

	if err := registry.ping(context.Background()); err == nil || !strings.Contains(err.Error(), "didn't answer; check that its container is running") {
		t.Errorf("ping error = %v, want the no-answer message", err)
	}
}

type recordingRefresher struct {
	mu  sync.Mutex
	ids []int64
}

func (r *recordingRefresher) StartBackgroundRefresh(id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ids = append(r.ids, id)

	return true, nil
}

func (r *recordingRefresher) started() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]int64(nil), r.ids...)
}

type pushFixture struct {
	*internalRegistryFixture
	registry  *InternalRegistry
	refresher *recordingRefresher
	imageID   int64
}

func newPushFixture(t *testing.T) *pushFixture {
	t.Helper()

	f := newInternalRegistryFixture(t)
	refresher := &recordingRefresher{}
	registry := NewInternalRegistry(InternalRegistryDeps{
		Store:     f.store,
		Audit:     f.audit,
		Failures:  stubFailures{},
		Refresher: refresher,
		Enabled:   true,
		Address:   "registry:5000",
	})

	if err := registry.EnsureInternal(f.ctx); err != nil {
		t.Fatalf("EnsureInternal: %v", err)
	}

	image := &model.Image{Name: "web", RegistryID: f.internal().ID, Repository: "team/web", Tag: "v1"}

	if err := f.store.CreateImage(f.ctx, image); err != nil {
		t.Fatalf("creating image: %v", err)
	}

	registry.quietPeriod = 50 * time.Millisecond
	registry.maxWait = time.Second

	return &pushFixture{internalRegistryFixture: f, registry: registry, refresher: refresher, imageID: image.ID}
}

func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)

	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestPushesToOneRepositoryCauseOneTagCheck(t *testing.T) {
	f := newPushFixture(t)

	f.registry.Pushed("team/web", "v2", "build", RequestInfo{IP: "10.0.0.9"})
	f.registry.Pushed("team/web", "latest", "build", RequestInfo{IP: "10.0.0.9"})
	f.registry.Pushed("team/other", "v1", "build", RequestInfo{IP: "10.0.0.9"})

	if got := f.audit.count(AuditTypeImagePushed); got != 0 {
		t.Fatalf("audits written during the request: %d, want 0", got)
	}

	waitFor(t, "the audit entries", func() bool { return f.audit.count(AuditTypeImagePushed) == 3 })
	waitFor(t, "the tag check", func() bool { return len(f.refresher.started()) == 1 })

	time.Sleep(100 * time.Millisecond)

	if got := f.refresher.started(); len(got) != 1 || got[0] != f.imageID {
		t.Errorf("tag checks = %v, want one for image %d", got, f.imageID)
	}
}

func TestPushesKeepWaitingButNotPastTheMaximum(t *testing.T) {
	f := newPushFixture(t)
	f.registry.maxWait = 200 * time.Millisecond
	start := time.Now()

	for time.Since(start) < 500*time.Millisecond {
		f.registry.Pushed("team/web", "v2", "build", RequestInfo{})
		time.Sleep(20 * time.Millisecond)
	}

	if got := f.refresher.started(); len(got) == 0 {
		t.Fatalf("no tag check while pushes kept arriving for 500ms with a 200ms maximum wait")
	}
}

func TestPushesFireAtThePendingLimit(t *testing.T) {
	f := newPushFixture(t)
	f.registry.quietPeriod = time.Hour
	f.registry.maxWait = time.Hour

	for range changeMaxPending {
		f.registry.Pushed("team/web", "v2", "build", RequestInfo{})
	}

	waitFor(t, "the flush at the pending limit", func() bool { return f.audit.count(AuditTypeImagePushed) == changeMaxPending })
}

func TestStopWritesPendingAuditsWithoutATagCheck(t *testing.T) {
	f := newPushFixture(t)
	f.registry.quietPeriod = time.Hour

	f.registry.Pushed("team/web", "v2", "build", RequestInfo{})
	f.registry.Stop()

	if got := f.audit.count(AuditTypeImagePushed); got != 1 {
		t.Errorf("audits after Stop = %d, want 1", got)
	}

	if got := f.refresher.started(); len(got) != 0 {
		t.Errorf("tag checks after Stop = %v, want none while shutting down", got)
	}
}

func TestDeletesAreAuditedAndStartATagCheck(t *testing.T) {
	f := newPushFixture(t)

	f.registry.Deleted("team/web", "sha256:"+strings.Repeat("a", 64), "cleanup", RequestInfo{IP: "10.0.0.9"})

	waitFor(t, "the delete audit", func() bool { return f.audit.count(AuditTypeManifestDeleted) == 1 })
	waitFor(t, "the tag check", func() bool { return len(f.refresher.started()) == 1 })

	if got := f.audit.count(AuditTypeImagePushed); got != 0 {
		t.Errorf("IMAGE_PUSHED audits = %d, want 0 for a delete", got)
	}
}

func TestInternalRepositoriesFollowsCatalogPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/_catalog" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/_catalog?last=team%2Fweb&n=100>; rel="next"`)
			io.WriteString(w, `{"repositories":["team/web","team/api"]}`)
			return
		}

		io.WriteString(w, `{"repositories":["base/alpine"]}`)
	}))
	defer server.Close()

	registry := NewInternalRegistry(InternalRegistryDeps{Enabled: true, Address: strings.TrimPrefix(server.URL, "http://")})

	got, err := registry.Repositories(context.Background())

	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	if want := []string{"base/alpine", "team/api", "team/web"}; !slices.Equal(got, want) {
		t.Errorf("repositories = %v, want %v", got, want)
	}
}

func TestInternalRepositoriesReportsNoAnswer(t *testing.T) {
	if _, err := NewInternalRegistry(InternalRegistryDeps{Enabled: true, Address: "127.0.0.1:1"}).Repositories(context.Background()); err == nil || !strings.Contains(err.Error(), "didn't answer") {
		t.Errorf("no answer: error = %v, want the no-answer message", err)
	}
}
