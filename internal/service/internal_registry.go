package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/store"
)

const (
	internalRegistryCheckInterval = 10 * time.Second
	internalRegistryCheckTimeout  = 5 * time.Second

	changeQuietPeriod  = 5 * time.Second
	changeMaxWait      = 30 * time.Second
	changeMaxPending   = 1000
	changeFlushTimeout = 10 * time.Second
)

var ErrRegistryManagedByConfig = errors.New("this registry is set by internal_registry in config; change it there")

var errInternalRegistryLogin = errors.New("asks for a login; remove its htpasswd settings, DockVMap handles logins")

type internalRegistryStore interface {
	GetInternalRegistry(ctx context.Context) (*model.RegistryInfo, error)
	GetRegistryInfoByHost(ctx context.Context, registry string) (*model.RegistryInfo, error)
	CreateRegistry(ctx context.Context, registry model.Registry) (int64, error)
	MarkRegistryInternal(ctx context.Context, registryID int64, address string, options model.RegistryOptions) error
	ClearInternalRegistry(ctx context.Context) error
	ListImageIDsByRegistryRepository(ctx context.Context, registryID int64, repository string) ([]int64, error)
}

type imageRefresher interface {
	StartBackgroundRefresh(imageID int64) (bool, error)
}

type auditImagePushedData struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Token      string `json:"token,omitempty"`
}

type auditManifestDeletedData struct {
	Repository string `json:"repository"`
	Reference  string `json:"reference"`
	Token      string `json:"token,omitempty"`
}

type pendingChange struct {
	reference, token string
	deleted          bool
	info             RequestInfo
}

type repositoryChanges struct {
	first   time.Time
	entries []pendingChange
	timer   *time.Timer
}

type InternalRegistryDeps struct {
	Store     internalRegistryStore
	Audit     auditRecorder
	Failures  failureRecorder
	Refresher imageRefresher
	Enabled   bool
	Address   string
	TLS       bool
}

type InternalRegistry struct {
	store         internalRegistryStore
	audit         auditRecorder
	failures      failureRecorder
	refresher     imageRefresher
	enabled       bool
	address       string
	useTLS        bool
	client        *http.Client
	checkInterval time.Duration
	quietPeriod   time.Duration
	maxWait       time.Duration

	mu      sync.Mutex
	pending map[string]*repositoryChanges
}

func NewInternalRegistry(deps InternalRegistryDeps) *InternalRegistry {
	return &InternalRegistry{
		store:         deps.Store,
		audit:         deps.Audit,
		failures:      deps.Failures,
		refresher:     deps.Refresher,
		enabled:       deps.Enabled,
		address:       deps.Address,
		useTLS:        deps.TLS,
		client:        &http.Client{Timeout: internalRegistryCheckTimeout},
		checkInterval: internalRegistryCheckInterval,
		quietPeriod:   changeQuietPeriod,
		maxWait:       changeMaxWait,
		pending:       make(map[string]*repositoryChanges),
	}
}

func (r *InternalRegistry) Address() string {
	if !r.enabled {
		return ""
	}

	return r.address
}

func (r *InternalRegistry) TLS() bool {
	return r.useTLS
}

func (r *InternalRegistry) url(path string) string {
	scheme := "http"

	if r.useTLS {
		scheme = "https"
	}

	return scheme + "://" + r.address + path
}

func (r *InternalRegistry) options() model.RegistryOptions {
	return model.RegistryOptions{Insecure: !r.useTLS}
}

func (r *InternalRegistry) EnsureInternal(ctx context.Context) error {
	marked, err := r.store.GetInternalRegistry(ctx)

	if err != nil {
		return err
	}

	if !r.enabled {
		if marked == nil {
			return nil
		}

		if err := r.store.ClearInternalRegistry(ctx); err != nil {
			return err
		}

		slog.Info("internal registry turned off; it stays as a normal registry", "registry", marked.Registry)
		r.auditRegistryEntry(ctx, AuditTypeRegistryUpdated, marked.Registry)

		return nil
	}

	if _, port, err := net.SplitHostPort(r.address); err != nil || port == "" || !validRegistryAddress(r.address) {
		return errors.New("internal_registry.address must be host:port, like registry:5000")
	}

	existing, err := r.store.GetRegistryInfoByHost(ctx, r.address)

	if err != nil {
		return err
	}

	switch {
	case marked != nil && marked.Registry == r.address && marked.Options == r.options():
		return nil

	case marked != nil && marked.Registry == r.address:
		if err := r.mark(ctx, marked.ID); err != nil {
			return err
		}

		slog.Info("internal registry connection changed", "registry", r.address, "tls", r.useTLS)
		r.auditRegistryEntry(ctx, AuditTypeRegistryUpdated, r.address)

	case marked != nil && existing != nil:
		return r.addressInUse()

	case marked != nil:
		if err := r.mark(ctx, marked.ID); err != nil {
			return err
		}

		slog.Info("internal registry moved to a new address; its virtual images follow", "from", marked.Registry, "to", r.address)
		r.auditRegistryEntry(ctx, AuditTypeRegistryUpdated, r.address)

	case existing != nil:
		if err := r.mark(ctx, existing.ID); err != nil {
			return err
		}

		slog.Info("took over the existing registry as the internal registry", "registry", r.address)
		r.auditRegistryEntry(ctx, AuditTypeRegistryUpdated, r.address)

	default:
		id, err := r.store.CreateRegistry(ctx, model.Registry{Registry: r.address})

		if err != nil {
			return err
		}

		if err := r.mark(ctx, id); err != nil {
			return err
		}

		slog.Info("added the internal registry", "registry", r.address)
		r.auditRegistryEntry(ctx, AuditTypeRegistryCreated, r.address)
	}

	return nil
}

func (r *InternalRegistry) mark(ctx context.Context, registryID int64) error {
	err := r.store.MarkRegistryInternal(ctx, registryID, r.address, r.options())

	if errors.Is(err, store.ErrRegistryConflict) {
		return r.addressInUse()
	}

	return err
}

func (r *InternalRegistry) addressInUse() error {
	return fmt.Errorf("internal_registry.address %s is already used by another registry in DockVMap; rename or delete that registry first", r.address)
}

func (r *InternalRegistry) auditRegistryEntry(ctx context.Context, auditType, registry string) {
	recordAudit(ctx, r.audit, auditType, auditRegistryData{
		Registry: registry,
		Options:  r.options(),
	})
}

func (r *InternalRegistry) StartChecking(ctx context.Context) {
	if r.enabled {
		go r.checkUntilReachable(ctx)
	}
}

func (r *InternalRegistry) checkUntilReachable(ctx context.Context) {
	failed := ""

	for {
		err := r.ping(ctx)

		if ctx.Err() != nil {
			return
		}

		if err == nil {
			if failed == "" {
				slog.Info("internal registry reachable", "registry", r.address)
			} else {
				slog.Info("internal registry answers again", "registry", r.address)
			}

			return
		}

		if err.Error() != failed {
			slog.Warn("internal registry check failed", "registry", r.address, "error", err)
			r.failures.Record(ctx, FailureSourceInternalRegistry, r.address, err)
			failed = err.Error()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(r.checkInterval):
		}
	}
}

func (r *InternalRegistry) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url("/v2/"), nil)

	if err != nil {
		return err
	}

	resp, err := r.client.Do(req)

	if problem := InternalRegistryCertProblem(err); problem != nil {
		return fmt.Errorf("has a certificate DockVMap doesn't accept: %w", problem)
	}

	if err != nil {
		return errors.New("didn't answer; check that its container is running on DockVMap's network")
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return errInternalRegistryLogin
	}

	return fmt.Errorf("answered %s instead of a registry; check internal_registry.address and internal_registry.tls", resp.Status)
}

func (r *InternalRegistry) Pushed(repository, tag, token string, info RequestInfo) {
	r.queue(repository, pendingChange{reference: tag, token: token, info: info})
}

func (r *InternalRegistry) Deleted(repository, reference, token string, info RequestInfo) {
	r.queue(repository, pendingChange{reference: reference, token: token, deleted: true, info: info})
}

func (r *InternalRegistry) queue(repository string, change pendingChange) {
	r.mu.Lock()
	defer r.mu.Unlock()

	changes, ok := r.pending[repository]

	if !ok {
		changes = &repositoryChanges{first: time.Now()}
		r.pending[repository] = changes
	}

	changes.entries = append(changes.entries, change)

	if len(changes.entries) >= changeMaxPending {
		r.takeLocked(repository)
		go r.apply(repository, changes.entries)

		return
	}

	wait := min(r.quietPeriod, max(0, r.maxWait-time.Since(changes.first)))

	if changes.timer == nil {
		changes.timer = time.AfterFunc(wait, func() { r.fire(repository) })
	} else {
		changes.timer.Reset(wait)
	}
}

func (r *InternalRegistry) fire(repository string) {
	r.mu.Lock()
	changes := r.takeLocked(repository)
	r.mu.Unlock()

	if changes != nil {
		r.apply(repository, changes.entries)
	}
}

func (r *InternalRegistry) takeLocked(repository string) *repositoryChanges {
	changes, ok := r.pending[repository]

	if !ok {
		return nil
	}

	delete(r.pending, repository)

	if changes.timer != nil {
		changes.timer.Stop()
	}

	return changes
}

func (r *InternalRegistry) Stop() {
	r.mu.Lock()

	taken := make(map[string]*repositoryChanges, len(r.pending))

	for repository := range r.pending {
		taken[repository] = r.takeLocked(repository)
	}

	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), changeFlushTimeout)
	defer cancel()

	for repository, changes := range taken {
		r.recordAudits(ctx, repository, changes.entries)
	}
}

func (r *InternalRegistry) apply(repository string, entries []pendingChange) {
	ctx, cancel := context.WithTimeout(context.Background(), changeFlushTimeout)
	defer cancel()

	r.recordAudits(ctx, repository, entries)
	r.checkTags(ctx, repository)
}

func (r *InternalRegistry) recordAudits(ctx context.Context, repository string, entries []pendingChange) {
	for _, change := range entries {
		changeCtx := WithRequestInfo(ctx, change.info)

		if change.deleted {
			recordAudit(changeCtx, r.audit, AuditTypeManifestDeleted, auditManifestDeletedData{
				Repository: repository,
				Reference:  change.reference,
				Token:      change.token,
			})

			continue
		}

		recordAudit(changeCtx, r.audit, AuditTypeImagePushed, auditImagePushedData{
			Repository: repository,
			Tag:        change.reference,
			Token:      change.token,
		})
	}
}

func (r *InternalRegistry) checkTags(ctx context.Context, repository string) {
	internal, err := r.store.GetInternalRegistry(ctx)

	if err != nil || internal == nil {
		slog.Error("finding the internal registry after a change failed", "repository", repository, "error", err)
		return
	}

	ids, err := r.store.ListImageIDsByRegistryRepository(ctx, internal.ID, repository)

	if err != nil {
		slog.Error("finding virtual images after a change failed", "repository", repository, "error", err)
		return
	}

	started := 0

	for _, id := range ids {
		ok, err := r.refresher.StartBackgroundRefresh(id)

		if err != nil {
			slog.Error("starting a tag check after a change failed", "repository", repository, "image", id, "error", err)
		}

		if ok {
			started++
		}
	}

	if started > 0 {
		slog.Info("checking tags after a change", "repository", repository, "images", started)
	}
}

func InternalRegistryCertProblem(err error) error {
	var certErr *tls.CertificateVerificationError

	if !errors.As(err, &certErr) {
		return nil
	}

	return certErr.Err
}
