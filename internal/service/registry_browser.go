package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/septi0/dockvmap/internal/model"
)

const (
	maxRegistryRepositories = 1000
	browsableCacheTTL       = time.Hour
	browsableCheckTimeout   = 5 * time.Second
)

type registryCatalog interface {
	ListRepositories(ctx context.Context, registry string, limit int) ([]string, bool, error)
	Forget(registry string)
}

type registryBrowserStore interface {
	GetRegistryInfoByID(ctx context.Context, registryID int64) (*model.RegistryInfo, error)
	ListRegistryInfo(ctx context.Context) ([]model.RegistryInfo, error)
	ListImagesByRegistry(ctx context.Context, registryID int64) ([]model.Image, error)
}

type RegistryBrowserDeps struct {
	Store      registryBrowserStore
	Catalog    registryCatalog
	Registries *Registries
}

type RegistryBrowser struct {
	store     registryBrowserStore
	catalog   registryCatalog
	browsable *expirable.LRU[int64, bool]
}

func NewRegistryBrowser(deps RegistryBrowserDeps) *RegistryBrowser {
	b := &RegistryBrowser{
		store:     deps.Store,
		catalog:   deps.Catalog,
		browsable: expirable.NewLRU[int64, bool](0, nil, browsableCacheTTL),
	}

	deps.Registries.OnChanged(b.forget)

	return b
}

func (b *RegistryBrowser) forget(registryID int64, hosts ...string) {
	b.browsable.Remove(registryID)

	for _, host := range hosts {
		b.catalog.Forget(host)
	}
}

func (b *RegistryBrowser) Browsable(ctx context.Context) ([]int64, error) {
	registries, err := b.store.ListRegistryInfo(ctx)

	if err != nil {
		return nil, err
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		ids = make([]int64, 0, len(registries))
	)

	for _, registry := range registries {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if b.isBrowsable(ctx, registry) {
				mu.Lock()
				ids = append(ids, registry.ID)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	slices.Sort(ids)

	return ids, nil
}

func (b *RegistryBrowser) isBrowsable(ctx context.Context, registry model.RegistryInfo) bool {
	if browsable, ok := b.browsable.Get(registry.ID); ok {
		return browsable
	}

	checkCtx, cancel := context.WithTimeout(ctx, browsableCheckTimeout)
	defer cancel()

	_, _, err := b.catalog.ListRepositories(checkCtx, registry.Registry, 1)

	if !isNetworkError(err) {
		b.browsable.Add(registry.ID, err == nil)
	}

	return err == nil
}

func isNetworkError(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr)
}

func (b *RegistryBrowser) Repositories(ctx context.Context, registryID int64) (*model.RegistryRepositories, error) {
	registry, err := b.store.GetRegistryInfoByID(ctx, registryID)

	if err != nil || registry == nil {
		return nil, err
	}

	names, more, err := b.catalog.ListRepositories(ctx, registry.Registry, maxRegistryRepositories)

	if err != nil {
		if !isNetworkError(err) {
			b.browsable.Remove(registryID)
		}

		slog.Warn("listing a registry's repositories failed", "registry", registry.Registry, "error", err)

		return nil, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	images, err := b.store.ListImagesByRegistry(ctx, registryID)

	if err != nil {
		return nil, err
	}

	byRepository := make(map[string][]model.Image)

	for _, image := range images {
		byRepository[image.Repository] = append(byRepository[image.Repository], image)
	}

	slices.Sort(names)

	result := &model.RegistryRepositories{Repositories: make([]model.RegistryRepository, 0, len(names)), More: more}

	for _, name := range names {
		result.Repositories = append(result.Repositories, model.RegistryRepository{Name: name, Images: byRepository[name]})
	}

	return result, nil
}
