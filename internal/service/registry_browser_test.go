package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/store"
)

type fakeCatalog struct {
	mu           sync.Mutex
	repositories map[string][]string
	errs         map[string]error
	calls        map[string]int
	forgotten    []string
}

func (c *fakeCatalog) ListRepositories(_ context.Context, registry string, limit int) ([]string, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls[registry]++

	if err := c.errs[registry]; err != nil {
		return nil, false, err
	}

	names := c.repositories[registry]

	return names[:min(limit, len(names))], len(names) > limit, nil
}

func (c *fakeCatalog) Forget(registry string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.forgotten = append(c.forgotten, registry)
}

func (c *fakeCatalog) callsFor(registry string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.calls[registry]
}

type browserFixture struct {
	ctx        context.Context
	store      *store.Store
	catalog    *fakeCatalog
	registries *Registries
	browser    *RegistryBrowser
}

func newBrowserFixture(t *testing.T) *browserFixture {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "test.db"), "")

	if err != nil {
		t.Fatalf("opening store: %v", err)
	}

	t.Cleanup(func() { st.Close() })

	refused := &oci.Error{StatusCode: http.StatusUnauthorized}
	catalog := &fakeCatalog{
		repositories: map[string][]string{},
		errs:         map[string]error{"docker.io": refused, "ghcr.io": refused},
		calls:        map[string]int{},
	}
	registries := NewRegistries(RegistriesDeps{Store: st, Audit: &recordingAudit{}})

	return &browserFixture{
		ctx:        context.Background(),
		store:      st,
		catalog:    catalog,
		registries: registries,
		browser:    NewRegistryBrowser(RegistryBrowserDeps{Store: st, Catalog: catalog, Registries: registries}),
	}
}

func (f *browserFixture) createRegistry(t *testing.T, host string) int64 {
	t.Helper()

	id, err := f.store.CreateRegistry(f.ctx, model.Registry{Registry: host})

	if err != nil {
		t.Fatalf("creating registry %s: %v", host, err)
	}

	return id
}

func (f *browserFixture) browsable(t *testing.T) []int64 {
	t.Helper()

	ids, err := f.browser.Browsable(f.ctx)

	if err != nil {
		t.Fatalf("Browsable: %v", err)
	}

	return ids
}

func TestBrowsableListsOnlyRegistriesThatAnswerTheCatalog(t *testing.T) {
	f := newBrowserFixture(t)

	ours := f.createRegistry(t, "registry.example.com")
	f.createRegistry(t, "refuses.example.com")
	f.createRegistry(t, "token.example.com")
	f.createRegistry(t, "down.example.com")

	f.catalog.errs["refuses.example.com"] = &oci.Error{StatusCode: http.StatusUnauthorized}
	f.catalog.errs["token.example.com"] = errors.New("token endpoint returned 400 Bad Request")
	f.catalog.errs["down.example.com"] = &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}

	if got := f.browsable(t); !slices.Equal(got, []int64{ours}) {
		t.Fatalf("browsable = %v, want only %d", got, ours)
	}

	f.browsable(t)

	for host, want := range map[string]int{"registry.example.com": 1, "refuses.example.com": 1, "token.example.com": 1, "down.example.com": 2} {
		if got := f.catalog.callsFor(host); got != want {
			t.Errorf("%s checked %d times, want %d (answers are cached, network errors aren't)", host, got, want)
		}
	}
}

func TestEditingARegistryChecksItAgain(t *testing.T) {
	f := newBrowserFixture(t)

	id := f.createRegistry(t, "harbor.example.com")
	f.catalog.errs["harbor.example.com"] = &oci.Error{StatusCode: http.StatusUnauthorized}

	if got := f.browsable(t); len(got) != 0 {
		t.Fatalf("browsable = %v, want none before the login", got)
	}

	delete(f.catalog.errs, "harbor.example.com")

	if _, err := f.registries.UpdateByID(f.ctx, model.RegistryUpdate{ID: id, Registry: "harbor.example.com", Options: &model.RegistryOptions{AllowSelfSignedCerts: true}}); err != nil {
		t.Fatalf("UpdateByID: %v", err)
	}

	if !slices.Contains(f.catalog.forgotten, "harbor.example.com") {
		t.Errorf("forgotten = %v, want the edited registry's cached login dropped", f.catalog.forgotten)
	}

	if got := f.browsable(t); !slices.Equal(got, []int64{id}) {
		t.Errorf("browsable after the edit = %v, want %d", got, id)
	}
}

func TestRepositoriesShowTheirVirtualImages(t *testing.T) {
	f := newBrowserFixture(t)

	id := f.createRegistry(t, "registry.example.com")
	other := f.createRegistry(t, "other.example.com")
	f.catalog.repositories["registry.example.com"] = []string{"team/web", "team/api", "base/alpine"}

	for _, image := range []model.Image{
		{Name: "web", RegistryID: id, Repository: "team/web", Tag: "v1"},
		{Name: "web-canary", RegistryID: id, Repository: "team/web", Tag: "v2"},
		{Name: "api-elsewhere", RegistryID: other, Repository: "team/api", Tag: "v1"},
	} {
		if err := f.store.CreateImage(f.ctx, &image); err != nil {
			t.Fatalf("creating image %s: %v", image.Name, err)
		}
	}

	result, err := f.browser.Repositories(f.ctx, id)

	if err != nil || result == nil {
		t.Fatalf("Repositories: %v, %v", result, err)
	}

	got := map[string][]string{}
	names := make([]string, 0)

	for _, repository := range result.Repositories {
		names = append(names, repository.Name)

		for _, image := range repository.Images {
			got[repository.Name] = append(got[repository.Name], image.Name)
		}
	}

	if !slices.Equal(names, []string{"base/alpine", "team/api", "team/web"}) || result.More {
		t.Errorf("repositories = %v, more %v; want sorted, all of them", names, result.More)
	}

	if !slices.Equal(got["team/web"], []string{"web", "web-canary"}) || len(got["team/api"]) != 0 {
		t.Errorf("images = %v, want web and web-canary on team/web only", got)
	}

	if missing, err := f.browser.Repositories(f.ctx, 999); err != nil || missing != nil {
		t.Errorf("unknown registry: %v, %v; want nil, nil", missing, err)
	}
}

func TestRepositoriesErrorStopsOfferingBrowse(t *testing.T) {
	f := newBrowserFixture(t)

	id := f.createRegistry(t, "registry.example.com")

	if got := f.browsable(t); len(got) != 1 {
		t.Fatalf("browsable = %v, want the registry", got)
	}

	f.catalog.errs["registry.example.com"] = &oci.Error{StatusCode: http.StatusForbidden}

	if _, err := f.browser.Repositories(f.ctx, id); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("error = %v, want ErrUpstreamUnavailable", err)
	}

	if got := f.browsable(t); len(got) != 0 {
		t.Errorf("browsable = %v, want none once the registry refuses", got)
	}
}

func TestRegistriesCountTheirImagesAndFilterByRegistry(t *testing.T) {
	f := newBrowserFixture(t)

	used := f.createRegistry(t, "registry.example.com")
	unused := f.createRegistry(t, "unused.example.com")

	for _, name := range []string{"web", "api"} {
		image := model.Image{Name: name, RegistryID: used, Repository: "team/" + name, Tag: "v1"}

		if err := f.store.CreateImage(f.ctx, &image); err != nil {
			t.Fatalf("creating image %s: %v", name, err)
		}

		if name == "api" {
			checkErr := "registry returned 401 Unauthorized"

			if _, err := f.store.UpdateImageCheck(f.ctx, nil, image.ID, &checkErr, time.Now(), ""); err != nil {
				t.Fatalf("recording a failed check: %v", err)
			}
		}
	}

	registries, err := f.store.ListRegistryInfo(f.ctx)

	if err != nil {
		t.Fatalf("ListRegistryInfo: %v", err)
	}

	counts := map[int64]int64{}
	failing := map[int64]int64{}
	hosts := make([]string, 0, len(registries))

	for _, registry := range registries {
		counts[registry.ID] = registry.ImageCount
		failing[registry.ID] = registry.FailedChecks
		hosts = append(hosts, registry.Registry)
	}

	if counts[used] != 2 || counts[unused] != 0 {
		t.Errorf("image counts = %v, want 2 for %d and 0 for %d", counts, used, unused)
	}

	if failing[used] != 1 || failing[unused] != 0 {
		t.Errorf("failed checks = %v, want 1 for %d and 0 for %d", failing, used, unused)
	}

	if !slices.IsSorted(hosts) {
		t.Errorf("registries = %v, want them sorted by name", hosts)
	}

	if info, err := f.store.GetRegistryInfoByID(f.ctx, used); err != nil || info.ImageCount != 2 {
		t.Errorf("GetRegistryInfoByID = %+v (%v), want an image count of 2", info, err)
	}

	for registryID, want := range map[int64]int{used: 2, unused: 0} {
		filters := model.ImageListFilters{RegistryID: registryID}
		filters.Limit = 10

		images, err := f.store.ListImages(f.ctx, filters)

		if err != nil || len(images) != want {
			t.Errorf("images of registry %d = %d (%v), want %d", registryID, len(images), err, want)
		}
	}
}
