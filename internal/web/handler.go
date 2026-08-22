package web

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/septi0/dockvmap/frontend"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/proxy"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/taganalyzer"
)

type imageService interface {
	Create(ctx context.Context, img model.Image) error
	Delete(ctx context.Context, imageId int64) (bool, error)
	GetByID(ctx context.Context, imageId int64) (*model.Image, error)
	GetTags(ctx context.Context, imageId int64) ([]model.ImageTag, error)
	List(ctx context.Context, filters model.ImageListFilters) ([]model.Image, error)
	Count(ctx context.Context, filters model.ImageListFilters) (int64, error)
	RefreshAvailableTags(ctx context.Context, imageId int64, opts service.RefreshTagsOpts) error
	UpdateTag(ctx context.Context, imageId int64, tag string) error
	InspectRepository(ctx context.Context, registry string, repository string) (taganalyzer.Analysis, error)
	MarkTagsAsSeen(ctx context.Context, imageId int64) (int64, error)
}

type registryService interface {
	Create(ctx context.Context, registry model.Registry) (int64, error)
	Update(ctx context.Context, registry model.RegistryUpdate) (bool, error)
	Delete(ctx context.Context, registryID int64) (bool, error)
	Get(ctx context.Context, registryID int64) (*model.RegistryInfo, error)
	List(ctx context.Context) ([]model.RegistryInfo, error)
}

type eventService interface {
	List(ctx context.Context, offset, limit int) ([]model.ImageEvent, error)
}

type auditService interface {
	List(ctx context.Context, filters model.AuditLogListFilters) ([]model.AuditLog, error)
	Count(ctx context.Context, filters model.AuditLogListFilters) (int64, error)
}

type userService interface {
	SetupRequired(ctx context.Context) (bool, error)
	Bootstrap(ctx context.Context, username, email, password string) (int64, error)
	UpdatePassword(ctx context.Context, currentPassword, newPassword string) error
	GetProfile(ctx context.Context) (*model.User, error)
	UpdateEmail(ctx context.Context, email string) error
	UpdatePreferences(ctx context.Context, patch model.UserPreferencesUpdate) error
}

type sessionService interface {
	Login(ctx context.Context, username, password string) (token string, expiresAt time.Time, err error)
	Logout(ctx context.Context, token string) error
	Validate(ctx context.Context, token string) (*model.CurrentUser, error)
	ListActive(ctx context.Context) ([]model.Session, error)
	InvalidateSession(ctx context.Context, sessionID int64) error
}

type healthChecker interface {
	Ping(ctx context.Context) error
}

type proxyTokenService interface {
	Create(ctx context.Context, label string) (int64, string, error)
	List(ctx context.Context) ([]model.ProxyToken, error)
	Delete(ctx context.Context, id int64) (bool, error)
}

type proxyMetricsProvider interface {
	Snapshot() proxy.MetricsSnapshot
}

type failureLister interface {
	Recent() []service.Failure
}

type Web struct {
	images               imageService
	registries           registryService
	events               eventService
	audit                auditService
	users                userService
	sessions             sessionService
	health               healthChecker
	proxyTokens          proxyTokenService
	proxyMetrics         proxyMetricsProvider
	failures             failureLister
	cfg                  *config.Config
	trustedProxies       ipmatch.Set
	loginRateLimitWindow time.Duration
	version              string
}

func New(cfg *config.Config, images imageService, registries registryService, events eventService, audit auditService, users userService, sessions sessionService, health healthChecker, proxyTokens proxyTokenService, proxyMetrics proxyMetricsProvider, failures failureLister, loginRateLimitWindow time.Duration, version string) (http.Handler, error) {
	trustedProxies, err := ipmatch.Parse(cfg.TrustedProxies)

	if err != nil {
		return nil, fmt.Errorf("parsing trusted_proxies: %w", err)
	}

	w := &Web{
		images:               images,
		registries:           registries,
		events:               events,
		audit:                audit,
		users:                users,
		sessions:             sessions,
		health:               health,
		proxyTokens:          proxyTokens,
		proxyMetrics:         proxyMetrics,
		failures:             failures,
		cfg:                  cfg,
		trustedProxies:       trustedProxies,
		version:              version,
		loginRateLimitWindow: loginRateLimitWindow,
	}

	mux := http.NewServeMux()
	apiMux := http.NewServeMux()

	w.registerAPIRoutes(apiMux)

	mux.HandleFunc("/health", w.apiHealth)

	mux.Handle("/api/", http.StripPrefix("/api", w.withRequestInfo(w.requireAuth(apiMux))))

	w.registerFrontendRoutes(mux)

	return securityHeaders(mux), nil
}

func (w *Web) registerFrontendRoutes(mux *http.ServeMux) {
	static, err := fs.Sub(frontend.FS, "dist")

	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(static))

	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		if _, err := fs.Stat(static, path); err != nil {
			serveSPA(rw, r)
			return
		}

		fileServer.ServeHTTP(rw, r)
	})
}

func serveSPA(w http.ResponseWriter, r *http.Request) {
	data, err := frontend.FS.ReadFile("dist/index.html")

	if err != nil {
		http.Error(w, "frontend unavailable", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	_, _ = w.Write(data)
}
