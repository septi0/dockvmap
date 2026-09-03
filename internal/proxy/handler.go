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
	"github.com/septi0/dockvmap/internal/model"
	"github.com/septi0/dockvmap/internal/oci"
)

func setAccess(r *http.Request, mutate func(*httpmw.AccessFields)) {
	if fields := httpmw.AccessFieldsFrom(r.Context()); fields != nil {
		mutate(fields)
	}
}

type imageResolver interface {
	Resolve(ctx context.Context, name string) (*model.Image, error)
}

type tokenVerifier interface {
	Verify(ctx context.Context, token string) (bool, error)
}

type Proxy struct {
	cfg     *config.Config
	images  imageResolver
	client  *oci.Client
	cache   *blobcache.Cache
	metrics *Metrics
	tokens  tokenVerifier
}

func New(cfg *config.Config, images imageResolver, client *oci.Client, cache *blobcache.Cache, metrics *Metrics, tokens tokenVerifier) *Proxy {
	if metrics == nil {
		metrics = NewMetrics()
	}

	return &Proxy{
		cfg:     cfg,
		images:  images,
		client:  client,
		cache:   cache,
		metrics: metrics,
		tokens:  tokens,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.metrics.totalRequests.Add(1)

	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if p.cfg.ProxyAuth.Enabled {
		authenticated := p.authenticate(r)

		setAccess(r, func(f *httpmw.AccessFields) {
			f.AuthenticateSeen = true
			f.Authenticated = authenticated
		})

		if !authenticated {
			w.Header().Set("Www-Authenticate", `Basic realm="dockvmap"`)
			writeOCIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")

			return
		}
	}

	path := r.URL.Path

	if path == "/v2/" || path == "/v2" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
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

func (p *Proxy) authenticate(r *http.Request) bool {
	if !p.cfg.ProxyAuth.Enabled {
		return true
	}

	_, password, ok := r.BasicAuth()

	if !ok {
		return false
	}

	valid, err := p.tokens.Verify(r.Context(), password)

	if err != nil {
		slog.Error("proxy token verification failed", "error", err)
		return false
	}

	return valid
}

func (p *Proxy) handleManifest(w http.ResponseWriter, r *http.Request, name, reference string) {
	img, err := p.images.Resolve(r.Context(), name)

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

	if p.cache != nil && blobcache.IsDigest(upstreamRef) {
		served, err := p.cache.Serve(r.Context(), w, r, upstreamRef)

		if err != nil {
			slog.Warn("manifest cache lookup failed", "digest", upstreamRef, "error", err)
		}

		if served {
			p.metrics.cacheHits.Add(1)
			setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "hit" })
			slog.Debug("manifest served from cache", "method", r.Method, "name", name, "reference", reference)
			return
		}

		p.metrics.cacheMisses.Add(1)
		setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "miss" })
	}

	slog.Debug("manifest fetched from upstream", "method", r.Method, "name", name, "reference", reference)
	p.proxyRequest(w, r, upstreamURL, upstreamRef, "manifest", img.Registry, path)
}

func (p *Proxy) handleBlob(w http.ResponseWriter, r *http.Request, name, digest string) {
	img, err := p.images.Resolve(r.Context(), name)

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

	if p.cache != nil {
		served, err := p.cache.Serve(r.Context(), w, r, digest)

		if err != nil {
			slog.Warn("blob cache lookup failed", "digest", digest, "error", err)
		}

		if served {
			p.metrics.cacheHits.Add(1)
			setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "hit" })
			slog.Debug("blob served from cache", "method", r.Method, "name", name, "digest", digest)
			return
		}

		p.metrics.cacheMisses.Add(1)
		setAccess(r, func(f *httpmw.AccessFields) { f.Cache = "miss" })
	}

	slog.Debug("blob fetched from upstream", "method", r.Method, "name", name, "digest", digest, "registry", img.Registry, "repository", path)

	p.proxyRequest(w, r, upstreamURL, digest, "blob", img.Registry, path)
}

func (p *Proxy) proxyRequest(w http.ResponseWriter, r *http.Request, upstreamURL, digest, resource, registryName, repository string) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, nil)

	if err != nil {
		slog.Error("creating upstream request failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	for _, h := range []string{"Accept", "Accept-Encoding", "Range"} {
		for _, v := range r.Header.Values(h) {
			req.Header.Add(h, v)
		}
	}

	p.metrics.upstreamRequests.Add(1)
	resp, err := p.client.Do(req, registryName, repository)

	if err != nil {
		p.metrics.upstreamFailures.Add(1)
		slog.Error("upstream request failed", "error", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode != http.StatusNotFound {
			p.metrics.upstreamFailures.Add(1)
		}

		writeUpstreamOCIError(w, resp, resource)
		return
	}

	p.copyResponseHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	if r.Method == http.MethodHead {
		return
	}

	if p.cache != nil && resp.StatusCode == http.StatusOK && r.Header.Get("Range") == "" && resp.Header.Get("Content-Encoding") == "" {
		cacheDigest := resp.Header.Get("Docker-Content-Digest")

		if cacheDigest == "" {
			cacheDigest = digest
		}

		outcome := p.cache.StreamAndStore(r.Context(), w, cacheDigest, resp.Header.Get("Content-Type"), resp.Body)

		if outcome.CacheErr != nil {
			p.metrics.cacheWriteFailures.Add(1)
			slog.Warn("cache write failed", "resource", resource, "digest", cacheDigest, "error", outcome.CacheErr)
		}

		if outcome.CopyErr != nil {
			slog.Warn("upstream response stream failed", "error", outcome.CopyErr)
		}

		return
	}

	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("upstream response stream failed", "error", err)
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

func writeUpstreamOCIError(w http.ResponseWriter, response *http.Response, resource string) {
	var upstream ociError

	if err := json.NewDecoder(response.Body).Decode(&upstream); err == nil && len(upstream.Errors) > 0 {
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
