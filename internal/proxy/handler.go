package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/httpmw"
	"github.com/septi0/dockvmap/internal/ipmatch"
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/service"
)

func setAccess(r *http.Request, mutate func(*httpmw.AccessFields)) {
	if fields := httpmw.AccessFieldsFrom(r.Context()); fields != nil {
		mutate(fields)
	}
}

type imageResolver interface {
	Resolve(ctx context.Context, name string) (*model.Image, error)
	OnTagChanged(fn func(names ...string))
}

type tokenVerifier interface {
	Verify(ctx context.Context, token string) (*model.ProxyToken, error)
}

type usageRecorder interface {
	RecordImagePull(imageID int64, tag string)
	RecordTagDigest(imageID int64, tag, digest string)
}

type changeRecorder interface {
	Pushed(repository, tag, token string, info service.RequestInfo)
	Deleted(repository, reference, token string, info service.RequestInfo)
}

type Proxy struct {
	cfg          *config.Config
	images       imageResolver
	resolveCache *resolveCache
	client       *oci.Client
	cache        *blobcache.Cache
	metrics      *Metrics
	tokens       tokenVerifier
	usage        usageRecorder
	internal     *passthrough
	trusted      ipmatch.Set
}

type Deps struct {
	Config  *config.Config
	Images  imageResolver
	Client  *oci.Client
	Cache   *blobcache.Cache
	Metrics *Metrics
	Tokens  tokenVerifier
	Usage   usageRecorder

	InternalRegistry string
	Changes          changeRecorder
	TrustedProxies   ipmatch.Set
}

func New(deps Deps) *Proxy {
	metrics := deps.Metrics

	if metrics == nil {
		metrics = NewMetrics()
	}

	resolveCache := newResolveCache()

	deps.Images.OnTagChanged(resolveCache.invalidate)

	var internal *passthrough

	if deps.InternalRegistry != "" {
		internal = newPassthrough(deps.InternalRegistry, deps.Changes)
	}

	return &Proxy{
		internal:     internal,
		trusted:      deps.TrustedProxies,
		cfg:          deps.Config,
		images:       deps.Images,
		resolveCache: resolveCache,
		client:       deps.Client,
		cache:        deps.Cache,
		metrics:      metrics,
		tokens:       deps.Tokens,
		usage:        deps.Usage,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.internal == nil {
		p.metrics.totalRequests.Add(1)
		p.serveVirtual(w, r)

		return
	}

	if !p.serveWithInternal(w, r) {
		p.metrics.totalRequests.Add(1)
	}
}

func (p *Proxy) serveVirtual(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if _, ok := p.requireAuth(w, r); !ok {
		return
	}

	path := r.URL.Path

	if path == "/v2/" || path == "/v2" {
		writeBaseResponse(w)
		return
	}

	if !strings.HasPrefix(path, "/v2/") {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(strings.TrimPrefix(path, "/v2/"), "/")

	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}

	op := parts[len(parts)-2]
	ref := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-2], "/")

	switch op {
	case "manifests":
		p.metrics.manifestRequests.Add(1)
		p.handleManifest(w, r, name, ref)
	case "blobs":
		p.metrics.blobRequests.Add(1)
		p.handleBlob(w, r, name, ref)
	default:
		http.NotFound(w, r)
	}
}

func (p *Proxy) serveWithInternal(w http.ResponseWriter, r *http.Request) (forwarded bool) {
	controller := setReadDeadline(w)

	token, ok := p.requireAuth(w, r)

	if !ok {
		return false
	}

	if r.URL.Path == "/v2/" || r.URL.Path == "/v2" {
		writeBaseResponse(w)
		return false
	}

	parsed, ok := parseRegistryPath(r.URL.Path)

	if !ok {
		http.NotFound(w, r)
		return false
	}

	write := r.Method != http.MethodGet && r.Method != http.MethodHead

	if parsed.kind != pathCatalog {
		if !service.ValidRepositoryName(parsed.name) {
			writeOCIError(w, http.StatusBadRequest, "NAME_INVALID", fmt.Sprintf("%q isn't a valid repository name", parsed.name))
			return false
		}

		img, err := p.resolveCache.resolve(r.Context(), parsed.name, p.images.Resolve)

		if err != nil {
			slog.Error("image lookup failed", "method", r.Method, "name", parsed.name, "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return false
		}

		if img != nil && !(write && ownRepositoryOnInternal(img)) {
			p.serveVirtualPath(w, r, img, parsed, write)
			return false
		}

		if img != nil && r.Method == http.MethodPut && parsed.kind == pathManifest && parsed.ref == p.cfg.VirtualTag {
			writeOCIError(w, http.StatusMethodNotAllowed, "UNSUPPORTED", fmt.Sprintf("%s is the virtual tag DockVMap manages for %s; push another tag", parsed.ref, img.Name))
			return false
		}
	}

	if write && p.cfg.ProxyAuth.Enabled && !token.CanPush {
		writeOCIError(w, http.StatusForbidden, "DENIED", "this token can't push; create a token with Can push on the Proxy Tokens page")
		return false
	}

	setAccess(r, func(f *httpmw.AccessFields) {
		f.Registry = p.internal.address
		f.Repository = parsed.name
		f.Reference = parsed.ref
		f.Resource = "passthrough"
	})

	if write && r.Body != nil && r.Body != http.NoBody {
		r.Body = &uploadBody{ReadCloser: r.Body, controller: controller}
	}

	pushedTag := r.Method == http.MethodPut && parsed.kind == pathManifest && !oci.IsDigest(parsed.ref)
	deleted := r.Method == http.MethodDelete && parsed.kind == pathManifest

	if pushedTag || deleted {
		label := ""

		if token != nil {
			label = token.Label
		}

		r = r.WithContext(withChange(r.Context(), change{
			repository: parsed.name,
			reference:  parsed.ref,
			token:      label,
			deleted:    deleted,
			info:       service.RequestInfo{IP: httpmw.ClientIP(r, p.trusted), UserAgent: r.UserAgent()},
		}))
	}

	p.internal.proxy.ServeHTTP(w, r)

	return true
}

func ownRepositoryOnInternal(img *model.Image) bool {
	return img.RegistryInternal && img.Repository == img.Name
}

func (p *Proxy) serveVirtualPath(w http.ResponseWriter, r *http.Request, img *model.Image, parsed registryPath, write bool) {
	if write {
		writeOCIError(w, http.StatusMethodNotAllowed, "UNSUPPORTED", fmt.Sprintf("%s is a virtual image and can't be pushed to; push the real repository instead, like %s", img.Name, img.Repository))
		return
	}

	switch parsed.kind {
	case pathManifest:
		p.metrics.manifestRequests.Add(1)
		p.handleManifest(w, r, parsed.name, parsed.ref)
	case pathBlob:
		p.metrics.blobRequests.Add(1)
		p.handleBlob(w, r, parsed.name, parsed.ref)
	default:
		http.NotFound(w, r)
	}
}

func (p *Proxy) requireAuth(w http.ResponseWriter, r *http.Request) (*model.ProxyToken, bool) {
	if !p.cfg.ProxyAuth.Enabled {
		return nil, true
	}

	token := p.authenticate(r)

	setAccess(r, func(f *httpmw.AccessFields) {
		f.AuthenticateSeen = true
		f.Authenticated = token != nil
	})

	if token == nil {
		w.Header().Set("Www-Authenticate", `Basic realm="dockvmap"`)
		writeOCIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")

		return nil, false
	}

	return token, true
}

func writeBaseResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func (p *Proxy) authenticate(r *http.Request) *model.ProxyToken {
	_, password, ok := r.BasicAuth()

	if !ok {
		return nil
	}

	token, err := p.tokens.Verify(r.Context(), password)

	if err != nil {
		slog.Error("proxy token verification failed", "error", err)
		return nil
	}

	return token
}

func (p *Proxy) handleManifest(w http.ResponseWriter, r *http.Request, name, reference string) {
	img, err := p.resolveCache.resolve(r.Context(), name, p.images.Resolve)

	if err != nil {
		slog.Error("manifest lookup failed", "method", r.Method, "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	if img == nil {
		slog.Info("manifest virtual image not found", "method", r.Method, "name", name)
		writeOCIError(w, http.StatusNotFound, "NAME_UNKNOWN", fmt.Sprintf("virtual image %q not configured", name))

		return
	}

	upstreamRef := reference

	if reference == p.cfg.VirtualTag {
		upstreamRef = img.Tag
	}

	if upstreamRef == "" {
		slog.Warn("manifest has no upstream tag", "method", r.Method, "name", name, "virtual_tag", p.cfg.VirtualTag)
		writeOCIError(w, http.StatusNotFound, "TAG_UNKNOWN", fmt.Sprintf("virtual image %q has no upstream tag configured for virtual tag %q", name, p.cfg.VirtualTag))
		return
	}

	host := oci.RegistryAPIHost(img.Registry)
	path := oci.RepositoryPath(img.Registry, img.Repository)
	upstreamURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", host, path, upstreamRef)

	setAccess(r, func(f *httpmw.AccessFields) {
		f.VirtualImage = name
		f.Registry = img.Registry
		f.Repository = path
		f.Reference = reference
		f.UpstreamRef = upstreamRef
		f.Resource = "manifest"
	})

	slog.Debug("manifest request", "method", r.Method, "name", name, "reference", reference, "registry", img.Registry, "repository", path, "upstream_reference", upstreamRef)

	cache := p.cacheFor(img)

	if cache != nil && oci.IsDigest(upstreamRef) {
		served, err := cache.Serve(r.Context(), w, r, upstreamRef)

		if err != nil {
			slog.Warn("manifest cache lookup failed", "digest", upstreamRef, "error", err)
		}

		if served {
			p.metrics.cacheHits.Add(1)
			setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "hit" })
			slog.Debug("manifest served from cache", "method", r.Method, "name", name, "reference", reference)
			p.recordPull(r, img, upstreamRef)
			return
		}

		p.metrics.cacheMisses.Add(1)
		setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "miss" })
	}

	slog.Debug("manifest fetched from upstream", "method", r.Method, "name", name, "reference", reference)

	if p.proxyRequest(w, r, img, cache, upstreamURL, upstreamRef, "manifest", path) {
		p.recordPull(r, img, upstreamRef)
		p.recordTagDigest(w, r, img, upstreamRef)
	}
}

func (p *Proxy) recordTagDigest(w http.ResponseWriter, r *http.Request, img *model.Image, upstreamRef string) {
	digest := w.Header().Get("Docker-Content-Digest")

	if upstreamRef != img.Tag || !oci.IsDigest(digest) || !oci.AcceptsIndex(r.Header.Values("Accept")) {
		return
	}

	if known, _ := img.TagDigest(); known != nil && *known == digest {
		return
	}

	p.usage.RecordTagDigest(img.ID, img.Tag, digest)
}

func (p *Proxy) recordPull(r *http.Request, img *model.Image, upstreamRef string) {
	if r.Method != http.MethodGet {
		return
	}

	tag := upstreamRef

	if oci.IsDigest(upstreamRef) {
		tag = img.Tag
	}

	p.usage.RecordImagePull(img.ID, tag)
}

func (p *Proxy) handleBlob(w http.ResponseWriter, r *http.Request, name, digest string) {
	img, err := p.resolveCache.resolve(r.Context(), name, p.images.Resolve)

	if err != nil {
		slog.Error("blob lookup failed", "method", r.Method, "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if img == nil {
		slog.Info("blob virtual image not found", "method", r.Method, "name", name)
		writeOCIError(w, http.StatusNotFound, "NAME_UNKNOWN", fmt.Sprintf("virtual image %q not configured", name))
		return
	}

	host := oci.RegistryAPIHost(img.Registry)
	path := oci.RepositoryPath(img.Registry, img.Repository)
	upstreamURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", host, path, digest)

	setAccess(r, func(f *httpmw.AccessFields) {
		f.VirtualImage = name
		f.Registry = img.Registry
		f.Repository = path
		f.Reference = digest
		f.Resource = "blob"
	})

	cache := p.cacheFor(img)

	if cache != nil {
		served, err := cache.Serve(r.Context(), w, r, digest)

		if err != nil {
			slog.Warn("blob cache lookup failed", "digest", digest, "error", err)
		}

		if served {
			p.metrics.cacheHits.Add(1)
			setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "hit" })
			slog.Debug("blob served from cache", "method", r.Method, "name", name, "digest", digest)
			p.recordPull(r, img, digest)
			return
		}

		p.metrics.cacheMisses.Add(1)
		setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "miss" })
	}

	slog.Debug("blob fetched from upstream", "method", r.Method, "name", name, "digest", digest, "registry", img.Registry, "repository", path)

	if p.proxyRequest(w, r, img, cache, upstreamURL, digest, "blob", path) {
		p.recordPull(r, img, digest)
	}
}

func (p *Proxy) cacheFor(img *model.Image) *blobcache.Cache {
	if img.RegistryInternal {
		return nil
	}

	return p.cache
}

func (p *Proxy) proxyRequest(w http.ResponseWriter, r *http.Request, img *model.Image, cache *blobcache.Cache, upstreamURL, digest, resource, repository string) bool {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, nil)

	if err != nil {
		slog.Error("creating upstream request failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return false
	}

	for _, h := range []string{"Accept", "Accept-Encoding", "Range"} {
		for _, v := range r.Header.Values(h) {
			req.Header.Add(h, v)
		}
	}

	p.metrics.upstreamRequests.Add(1)
	resp, err := p.client.Do(req, img.Registry, repository)

	if err != nil {
		p.metrics.upstreamFailures.Add(1)
		slog.Error("upstream request failed", "registry", img.Registry, "error", err)
		writeOCIError(w, http.StatusBadGateway, "UNKNOWN", upstreamFailureMessage(img, err))
		return false
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode != http.StatusNotFound {
			p.metrics.upstreamFailures.Add(1)
		}

		writeUpstreamOCIError(w, resp, resource)
		return false
	}

	p.copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	if r.Method == http.MethodHead {
		return true
	}

	if cache != nil && resp.StatusCode == http.StatusOK && r.Header.Get("Range") == "" && resp.Header.Get("Content-Encoding") == "" {
		cacheDigest := resp.Header.Get("Docker-Content-Digest")

		if cacheDigest == "" {
			cacheDigest = digest
		}

		outcome := cache.StreamAndStore(r.Context(), w, cacheDigest, resp.Header.Get("Content-Type"), resp.Body)

		if outcome.CacheErr != nil {
			p.metrics.cacheWriteFailures.Add(1)
			slog.Warn("cache write failed", "resource", resource, "digest", cacheDigest, "error", outcome.CacheErr)
		}

		if outcome.CopyErr != nil {
			slog.Warn("upstream response stream failed", "error", outcome.CopyErr)
		}

		return outcome.CopyErr == nil
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("upstream response stream failed", "error", err)

		return false
	}

	return true
}

func upstreamFailureMessage(img *model.Image, err error) string {
	switch {
	case !isDialError(err):
		return fmt.Sprintf("the request to the registry at %s failed; check DockVMap's log", img.Registry)
	case img.RegistryInternal:
		return internalRegistryUnreachable(img.Registry)
	default:
		return fmt.Sprintf("the registry at %s didn't answer; try again later", img.Registry)
	}
}

func (p *Proxy) copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	for _, h := range []string{
		"Content-Type", "Content-Length", "Docker-Content-Digest",
		"Docker-Distribution-Api-Version", "Etag", "Last-Modified",
		"Content-Range", "Accept-Ranges", "Content-Encoding",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
}

type ociError struct {
	Errors []ociErrorDetail `json:"errors"`
}

type ociErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeOCIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ociError{
		Errors: []ociErrorDetail{{Code: code, Message: message}},
	})
}

const maxUpstreamErrorBody = 8 << 10

func writeUpstreamOCIError(w http.ResponseWriter, response *http.Response, resource string) {
	var upstream ociError

	if err := json.NewDecoder(io.LimitReader(response.Body, maxUpstreamErrorBody)).Decode(&upstream); err == nil && len(upstream.Errors) > 0 {
		writeOCIError(w, response.StatusCode, upstream.Errors[0].Code, upstream.Errors[0].Message)
		return
	}

	code := "UNKNOWN"
	message := "upstream registry request failed"

	switch response.StatusCode {
	case http.StatusUnauthorized:
		code = "UNAUTHORIZED"
		message = "authentication required by upstream registry"
	case http.StatusForbidden:
		code = "DENIED"
		message = "access denied by upstream registry"
	case http.StatusNotFound:
		if resource == "blob" {
			code = "BLOB_UNKNOWN"
			message = "blob not found in upstream registry"
		} else {
			code = "MANIFEST_UNKNOWN"
			message = "manifest not found in upstream registry"
		}
	case http.StatusTooManyRequests:
		code = "TOOMANYREQUESTS"
		message = "upstream registry rate limit exceeded"
	}

	writeOCIError(w, response.StatusCode, code, message)
}
