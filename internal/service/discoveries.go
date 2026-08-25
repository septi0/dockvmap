package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/taganalyzer"
)

var ErrTagDiscoveryNotFound = errors.New("tag discovery not found")

const (
	discoveryInlineWaitBudget   = 2 * time.Second
	discoveryInlineWaitInterval = 150 * time.Millisecond
	discoveryScanTimeout        = 10 * time.Minute
)

type discoveryRegistryLookup interface {
	GetRegistryInfoByHost(ctx context.Context, registry string) (*model.RegistryInfo, error)
}

type repositoryChecker interface {
	CheckRepository(ctx context.Context, registry, repository string) error
}

type progressiveTagLister interface {
	ListTagsWithProgress(ctx context.Context, registry, repository string, onPage func(tagsSoFar int)) ([]string, error)
}

type discoveryStore interface {
	GetTagDiscoveryByID(ctx context.Context, id int64) (*model.TagDiscovery, error)
	GetTagDiscoveryByRegistryRepo(ctx context.Context, registryID int64, repository string) (*model.TagDiscovery, error)
	StartOrGetTagDiscovery(ctx context.Context, registryID int64, repository string) (*model.TagDiscovery, bool, error)
	CompleteTagDiscovery(ctx context.Context, id int64, groups []model.TagDiscoveryGroup, tagCount int, rawTagCount int) error
	FailTagDiscovery(ctx context.Context, id int64, errMessage string) error
	RecordTagDiscoveryRefreshFailure(ctx context.Context, id int64, errMessage string) error
	MarkStaleRunningTagDiscoveriesAsFailed(ctx context.Context) (int64, error)
	DeleteOldTagDiscoveries(ctx context.Context, olderThan time.Time) (int64, error)
}

type keySet struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func newKeySet() *keySet {
	return &keySet{keys: make(map[string]struct{})}
}

func (s *keySet) tryLock(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.keys[key]; ok {
		return false
	}

	s.keys[key] = struct{}{}

	return true
}

func (s *keySet) unlock(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.keys, key)
}

func discoveryRefreshKey(registryID int64, repository string) string {
	return fmt.Sprintf("%d/%s", registryID, repository)
}

type discoveryProgress struct {
	mu   sync.Mutex
	seen map[int64]int
}

func newDiscoveryProgress() *discoveryProgress {
	return &discoveryProgress{seen: make(map[int64]int)}
}

func (p *discoveryProgress) set(id int64, count int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.seen[id] = count
}

func (p *discoveryProgress) get(id int64) (int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	count, ok := p.seen[id]

	return count, ok
}

func (p *discoveryProgress) clear(id int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.seen, id)
}

type Discoveries struct {
	store      discoveryStore
	registries discoveryRegistryLookup
	checker    repositoryChecker
	tagLister  progressiveTagLister
	tagFilter  tagFilterer
	failures   failureRecorder
	bgCtx      context.Context
	ttl        time.Duration
	refreshing *keySet
	progress   *discoveryProgress
}

func NewDiscoveries(store discoveryStore, registries discoveryRegistryLookup, checker repositoryChecker, tagLister progressiveTagLister, tagFilter tagFilterer, failures failureRecorder, bgCtx context.Context, ttl time.Duration) *Discoveries {
	return &Discoveries{
		store:      store,
		registries: registries,
		checker:    checker,
		tagLister:  tagLister,
		tagFilter:  tagFilter,
		failures:   failures,
		bgCtx:      bgCtx,
		ttl:        ttl,
		refreshing: newKeySet(),
		progress:   newDiscoveryProgress(),
	}
}

func (d *Discoveries) withProgress(discovery *model.TagDiscovery) *model.TagDiscovery {
	if discovery != nil && discovery.Status == model.TagDiscoveryRunning {
		if seen, ok := d.progress.get(discovery.ID); ok {
			discovery.TagsSeen = seen
		}
	}

	return discovery
}

func (d *Discoveries) Check(ctx context.Context, registryHost, repository string) (model.TagDiscovery, error) {
	registryHost = strings.TrimSpace(registryHost)
	repository = strings.TrimSpace(repository)

	if !validRegistry(registryHost) {
		return model.TagDiscovery{}, fmt.Errorf("%w: registry must be a valid host", ErrInvalidImage)
	}

	if !repositoryNameRE.MatchString(repository) {
		return model.TagDiscovery{}, fmt.Errorf("%w: repository must be a valid lowercase repository path", ErrInvalidImage)
	}

	registryInfo, err := d.registries.GetRegistryInfoByHost(ctx, registryHost)

	if err != nil {
		return model.TagDiscovery{}, err
	}

	if registryInfo == nil {
		return model.TagDiscovery{}, fmt.Errorf("%w: registry does not exist", ErrInvalidImage)
	}

	if err := d.checker.CheckRepository(ctx, registryHost, repository); err != nil {
		var registryErr *oci.Error

		if errors.As(err, &registryErr) {
			switch registryErr.StatusCode {
			case http.StatusNotFound:
				return model.TagDiscovery{}, fmt.Errorf("%w: %s/%s", ErrUpstreamNotFound, registryHost, repository)

			case http.StatusUnauthorized:
				return model.TagDiscovery{}, fmt.Errorf("%w: %s/%s", ErrUpstreamUnauthorized, registryHost, repository)
			}
		}

		return model.TagDiscovery{}, fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}

	existing, err := d.store.GetTagDiscoveryByRegistryRepo(ctx, registryInfo.ID, repository)

	if err != nil {
		return model.TagDiscovery{}, err
	}

	if existing != nil && existing.Status == model.TagDiscoveryCompleted {
		if existing.CompletedAt == nil || time.Since(*existing.CompletedAt) >= d.ttl {
			d.maybeRefresh(registryInfo.ID, registryHost, repository, existing.ID)
		}

		return *existing, nil
	}

	discovery, started, err := d.store.StartOrGetTagDiscovery(ctx, registryInfo.ID, repository)

	if err != nil {
		return model.TagDiscovery{}, err
	}

	if started {
		d.spawnScan(discovery.ID, registryHost, repository)
	}

	return d.waitBriefly(ctx, discovery.ID), nil
}

func (d *Discoveries) Get(ctx context.Context, id int64) (*model.TagDiscovery, error) {
	if id < 1 {
		return nil, fmt.Errorf("%w: id must be positive", ErrInvalidImage)
	}

	discovery, err := d.store.GetTagDiscoveryByID(ctx, id)

	if err != nil {
		return nil, err
	}

	if discovery == nil {
		return nil, ErrTagDiscoveryNotFound
	}

	return d.withProgress(discovery), nil
}

func (d *Discoveries) CachedTags(ctx context.Context, registryID int64, repository string) ([]string, bool) {
	repository = strings.TrimSpace(repository)

	discovery, err := d.store.GetTagDiscoveryByRegistryRepo(ctx, registryID, repository)

	if err != nil {
		slog.Error("looking up cached tag discovery failed", "registryId", registryID, "repository", repository, "error", err)

		return nil, false
	}

	if discovery == nil || discovery.Status != model.TagDiscoveryCompleted {
		return nil, false
	}

	return flattenTagDiscoveryGroups(discovery.TagGroups), true
}

func (d *Discoveries) RecoverFromRestart(ctx context.Context) error {
	count, err := d.store.MarkStaleRunningTagDiscoveriesAsFailed(ctx)

	if err != nil {
		return fmt.Errorf("recovering tag discoveries: %w", err)
	}

	if count > 0 {
		slog.Warn("marked interrupted tag discoveries as failed", "count", count)
	}

	return nil
}

func (d *Discoveries) CleanupOld(ctx context.Context, retention time.Duration) (int64, error) {
	return d.store.DeleteOldTagDiscoveries(ctx, time.Now().UTC().Add(-retention))
}

func (d *Discoveries) waitBriefly(ctx context.Context, discoveryID int64) model.TagDiscovery {
	deadlineCtx, cancel := context.WithTimeout(ctx, discoveryInlineWaitBudget)
	defer cancel()

	ticker := time.NewTicker(discoveryInlineWaitInterval)
	defer ticker.Stop()

	for {
		current, err := d.store.GetTagDiscoveryByID(context.Background(), discoveryID)

		if err != nil {
			slog.Error("checking tag discovery status failed", "id", discoveryID, "error", err)
		} else if current != nil && current.Status != model.TagDiscoveryRunning {
			return *current
		}

		select {
		case <-deadlineCtx.Done():
			if current != nil {
				return *d.withProgress(current)
			}

			return *d.withProgress(&model.TagDiscovery{ID: discoveryID, Status: model.TagDiscoveryRunning})

		case <-ticker.C:
		}
	}
}

func (d *Discoveries) spawnScan(discoveryID int64, registryHost, repository string) {
	go d.runScan(discoveryID, registryHost, repository, func(err error) {
		if failErr := d.store.FailTagDiscovery(context.Background(), discoveryID, err.Error()); failErr != nil {
			slog.Error("recording tag discovery failure failed", "registry", registryHost, "repository", repository, "error", failErr)
		}

		slog.Error("tag discovery failed", "registry", registryHost, "repository", repository, "error", err)
	})
}

func (d *Discoveries) maybeRefresh(registryID int64, registryHost, repository string, discoveryID int64) {
	key := discoveryRefreshKey(registryID, repository)

	if !d.refreshing.tryLock(key) {
		return
	}

	go func() {
		defer d.refreshing.unlock(key)

		d.runScan(discoveryID, registryHost, repository, func(err error) {
			if failErr := d.store.RecordTagDiscoveryRefreshFailure(context.Background(), discoveryID, err.Error()); failErr != nil {
				slog.Error("recording tag discovery refresh failure failed", "registry", registryHost, "repository", repository, "error", failErr)
			}

			d.failures.Record(FailureSourceDiscoveryRefresh, repository, err)

			slog.Error("background tag discovery refresh failed", "registry", registryHost, "repository", repository, "error", err)
		})
	}()
}

func (d *Discoveries) runScan(discoveryID int64, registryHost, repository string, onFailure func(error)) {
	defer func() {
		if r := recover(); r != nil {
			onFailure(fmt.Errorf("panic during tag discovery: %v", r))
		}
	}()

	defer d.progress.clear(discoveryID)

	ctx, cancel := context.WithTimeout(d.bgCtx, discoveryScanTimeout)
	defer cancel()

	tags, err := d.tagLister.ListTagsWithProgress(ctx, registryHost, repository, func(tagsSoFar int) {
		d.progress.set(discoveryID, tagsSoFar)
	})

	if err != nil {
		onFailure(err)

		return
	}

	filtered := d.tagFilter.Apply(tags)
	analysis := taganalyzer.AnalyzeWithOptions(filtered, taganalyzer.AnalysisOptions{IncludeTokens: false})
	groups := tagDiscoveryGroupsFromAnalysis(analysis)

	if err := d.store.CompleteTagDiscovery(context.Background(), discoveryID, groups, len(filtered), len(tags)); err != nil {
		slog.Error("recording tag discovery result failed", "registry", registryHost, "repository", repository, "error", err)
	}
}

func tagDiscoveryGroupsFromAnalysis(analysis taganalyzer.Analysis) []model.TagDiscoveryGroup {
	prerelease := make(map[string]bool, len(analysis.Tags))
	for _, tag := range analysis.Tags {
		prerelease[tag.Tag] = taganalyzer.IsPrerelease(tag)
	}

	groups := make([]model.TagDiscoveryGroup, 0, len(analysis.Ordered))

	for _, family := range analysis.Ordered {
		tags := make([]model.TagDiscoveryTag, 0, len(family.OrderedTags))

		for _, tag := range family.OrderedTags {
			tags = append(tags, model.TagDiscoveryTag{
				Tag:        tag,
				Prerelease: prerelease[tag],
			})
		}

		groups = append(groups, model.TagDiscoveryGroup{
			FamilyID:   int64(family.ID),
			FamilyType: string(family.Kind),
			Tags:       tags,
		})
	}

	return groups
}

func flattenTagDiscoveryGroups(groups []model.TagDiscoveryGroup) []string {
	tags := make([]string, 0)

	for _, group := range groups {
		for _, tag := range group.Tags {
			tags = append(tags, tag.Tag)
		}
	}

	return tags
}
