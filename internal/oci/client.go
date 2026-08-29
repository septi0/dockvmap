package oci

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
)

const registryDataCacheTTL = 30 * time.Second
const tokenCacheMaxEntries = 512
const tagsListPageSize = 1000
const maxRetryAttempts = 3
const maxRetryAfterDelay = 30 * time.Second

var paramRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

type Credentials struct {
	Username   string
	Credential string
}

type RegistryOptions struct {
	Insecure             bool
	AllowSelfSignedCerts bool
}

type credentialProvider interface {
	GetRegistryCredentials(ctx context.Context, registry string) (*Credentials, error)
}

type registryOptionsProvider interface {
	GetRegistryOptions(ctx context.Context, registry string) (*RegistryOptions, error)
}

type Client struct {
	httpClient  *http.Client
	credentials credentialProvider
	options     registryOptionsProvider
	cache       *registryCache
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

// per-registry-host caches; each read is singleflight-guarded so a burst of misses triggers one fetch
type registryCache struct {
	tokens           *expirable.LRU[string, cachedToken]
	tokenFetch       singleflight.Group
	credentials      *expirable.LRU[string, *Credentials]
	credentialsFetch singleflight.Group
	options          *expirable.LRU[string, *RegistryOptions]
	optionsFetch     singleflight.Group
	insecureClients  *expirable.LRU[string, *http.Client]
}

func newRegistryCache() *registryCache {
	return &registryCache{
		tokens:          expirable.NewLRU[string, cachedToken](tokenCacheMaxEntries, nil, 0),
		credentials:     expirable.NewLRU[string, *Credentials](0, nil, registryDataCacheTTL),
		options:         expirable.NewLRU[string, *RegistryOptions](0, nil, registryDataCacheTTL),
		insecureClients: expirable.NewLRU[string, *http.Client](0, nil, registryDataCacheTTL),
	}
}

func cachedFetch[V any](lru *expirable.LRU[string, V], sf *singleflight.Group, key string, fetch func() (V, error)) (V, error) {
	if v, ok := lru.Get(key); ok {
		return v, nil
	}

	v, err, _ := sf.Do(key, func() (any, error) {
		if v, ok := lru.Get(key); ok {
			return v, nil
		}

		v, err := fetch()

		if err != nil {
			return v, err
		}

		lru.Add(key, v)

		return v, nil
	})

	if err != nil {
		var zero V
		return zero, err
	}

	return v.(V), nil
}

func (rc *registryCache) credentialsFor(registry string, fetch func() (*Credentials, error)) (*Credentials, error) {
	return cachedFetch(rc.credentials, &rc.credentialsFetch, registry, fetch)
}

func (rc *registryCache) optionsFor(registry string, fetch func() (*RegistryOptions, error)) (*RegistryOptions, error) {
	return cachedFetch(rc.options, &rc.optionsFetch, registry, fetch)
}

func (rc *registryCache) token(key string) (string, bool) {
	token, ok := rc.tokens.Get(key)

	if !ok || !token.expiresAt.After(time.Now().Add(5*time.Second)) {
		rc.tokens.Remove(key)

		return "", false
	}

	return token.value, true
}

func (rc *registryCache) removeToken(key string) {
	rc.tokens.Remove(key)
}

func (rc *registryCache) fetchToken(key string, fetch func() (string, time.Time, error)) (string, error) {
	if v, ok := rc.token(key); ok {
		return v, nil
	}

	v, err, _ := rc.tokenFetch.Do(key, func() (any, error) {
		if v, ok := rc.token(key); ok {
			return v, nil
		}

		value, expiresAt, err := fetch()

		if err != nil {
			return "", err
		}

		rc.tokens.Add(key, cachedToken{value: value, expiresAt: expiresAt})

		return value, nil
	})

	if err != nil {
		return "", err
	}

	return v.(string), nil
}

func (rc *registryCache) insecureClient(registry string, build func() *http.Client) *http.Client {
	if client, ok := rc.insecureClients.Get(registry); ok {
		return client
	}

	client := build()
	rc.insecureClients.Add(registry, client)

	return client
}

type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	return e.Message
}

func NewClient(httpClient *http.Client, credentials credentialProvider, options registryOptionsProvider) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: time.Second,
			},
		}
	}

	return &Client{
		httpClient:  httpClient,
		credentials: credentials,
		options:     options,
		cache:       newRegistryCache(),
	}
}

func (c *Client) ListTags(ctx context.Context, registry, repository string) ([]string, error) {
	return c.listTags(ctx, registry, repository, nil)
}

func (c *Client) ListTagsWithProgress(ctx context.Context, registry, repository string, onPage func(tagsSoFar int)) ([]string, error) {
	return c.listTags(ctx, registry, repository, onPage)
}

func (c *Client) listTags(ctx context.Context, registry, repository string, onPage func(tagsSoFar int)) ([]string, error) {
	host := RegistryAPIHost(registry)
	path := RepositoryPath(registry, repository)

	// scheme is a placeholder here; Do -> requestForRegistry rewrites it per registry Insecure option before sending
	endpoint := fmt.Sprintf("https://%s/v2/%s/tags/list?n=%d", host, path, tagsListPageSize)

	tags := make([]string, 0)

	for endpoint != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)

		if err != nil {
			return nil, fmt.Errorf("creating registry request: %w", err)
		}

		response, err := c.Do(req, registry, path)

		if err != nil {
			return nil, err
		}

		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			err := registryStatusError(response)

			drainAndClose(response)

			return nil, err
		}

		var page struct {
			Tags []string `json:"tags"`
		}

		decodeErr := json.NewDecoder(response.Body).Decode(&page)
		next := nextPageURL(response.Request.URL, response.Header.Get("Link"))

		drainAndClose(response)

		if decodeErr != nil {
			return nil, fmt.Errorf("decoding tag list from %s: %w", response.Request.URL.Host, decodeErr)
		}

		tags = append(tags, page.Tags...)

		if onPage != nil {
			onPage(len(tags))
		}

		endpoint = next
	}

	return tags, nil
}

func (c *Client) CheckRepository(ctx context.Context, registry, repository string) error {
	host := RegistryAPIHost(registry)
	path := RepositoryPath(registry, repository)

	endpoint := fmt.Sprintf("https://%s/v2/%s/tags/list", host, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)

	if err != nil {
		return fmt.Errorf("creating registry request: %w", err)
	}

	response, err := c.Do(req, registry, path)

	if err != nil {
		return err
	}

	defer drainAndClose(response)

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return registryStatusError(response)
	}

	return nil
}

func registryStatusError(response *http.Response) error {
	return &Error{
		StatusCode: response.StatusCode,
		Message:    fmt.Sprintf("registry returned %s", response.Status),
	}
}

// drainAndClose drains the body before closing so the connection can be reused for keep-alive.
func drainAndClose(response *http.Response) {
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func (c *Client) Do(req *http.Request, registry, repository string) (*http.Response, error) {
	options, err := c.registryOptions(req.Context(), registry)

	if err != nil {
		return nil, err
	}

	req = requestForRegistry(req, options)

	attempt := req
	preemptiveKey := ""

	if credentials, credErr := c.registryCredentials(req.Context(), registry); credErr == nil {
		key := c.tokenCacheKey(registry, repository, credentials)

		if token, ok := c.cache.token(key); ok {
			preemptiveKey = key
			attempt = req.Clone(req.Context())
			attempt.Header.Set("Authorization", "Bearer "+token)
		}
	}

	response, err := c.do(attempt, registry, options)

	if err != nil {
		return nil, fmt.Errorf("requesting registry: %w", err)
	}

	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}

	if preemptiveKey != "" {
		c.cache.removeToken(preemptiveKey)
	}

	challenge := response.Header.Get("Www-Authenticate")
	response.Body.Close()

	scheme, value := authenticationChallenge(challenge)

	switch strings.ToLower(scheme) {
	case "bearer":
		token, err := c.fetchBearerToken(req.Context(), value, registry, repository, options)

		if err != nil {
			return nil, err
		}

		retryReq := req.Clone(req.Context())
		retryReq.Header.Set("Authorization", "Bearer "+token)

		response, err = c.do(retryReq, registry, options)

		if err != nil {
			return nil, fmt.Errorf("retrying registry request with Bearer authentication: %w", err)
		}

		return response, nil

	case "basic":
		credentials, err := c.registryCredentials(req.Context(), registry)

		if err != nil {
			return nil, err
		}

		if credentials == nil {
			return nil, fmt.Errorf("registry requires Basic authentication but no credentials are configured")
		}

		retryReq := req.Clone(req.Context())
		retryReq.SetBasicAuth(credentials.Username, credentials.Credential)

		response, err = c.do(retryReq, registry, options)

		if err != nil {
			return nil, fmt.Errorf("retrying registry request with Basic authentication: %w", err)
		}

		return response, nil

	default:
		if scheme == "" {
			return nil, fmt.Errorf("registry returned an unsupported authentication challenge")
		}

		return nil, fmt.Errorf("registry returned unsupported authentication scheme %q", scheme)
	}
}

func (c *Client) fetchBearerToken(ctx context.Context, challenge string, registry, repository string, options *RegistryOptions) (string, error) {
	params := bearerParams(challenge)

	realm := params["realm"]

	if realm == "" {
		return "", fmt.Errorf("Bearer authentication challenge has no realm")
	}

	credentials, err := c.registryCredentials(ctx, registry)

	if err != nil {
		return "", err
	}

	cacheKey := c.tokenCacheKey(registry, repository, credentials)

	return c.cache.fetchToken(cacheKey, func() (string, time.Time, error) {
		return c.requestBearerToken(ctx, realm, registry, params, credentials, options)
	})
}

func (c *Client) requestBearerToken(ctx context.Context, realm, registry string, params map[string]string, credentials *Credentials, options *RegistryOptions) (string, time.Time, error) {
	tokenURL, err := url.Parse(realm)

	if err != nil {
		return "", time.Time{}, fmt.Errorf("parsing token realm: %w", err)
	}

	query := tokenURL.Query()

	if service := params["service"]; service != "" {
		query.Set("service", service)
	}

	if scope := params["scope"]; scope != "" {
		query.Set("scope", scope)
	}

	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)

	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating token request: %w", err)
	}

	if credentials != nil {
		req.SetBasicAuth(credentials.Username, credentials.Credential)
	}

	response, err := c.do(requestForRegistry(req, options), registry, options)

	if err != nil {
		return "", time.Time{}, fmt.Errorf("requesting registry token: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("token endpoint returned %s", response.Status)
	}

	var tokenResponse struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(response.Body).Decode(&tokenResponse); err != nil {
		return "", time.Time{}, fmt.Errorf("decoding token response: %w", err)
	}

	token := tokenResponse.Token

	if token == "" {
		token = tokenResponse.AccessToken
	}

	if token == "" {
		return "", time.Time{}, fmt.Errorf("token response did not contain a token")
	}

	expiresIn := time.Duration(tokenResponse.ExpiresIn) * time.Second

	if expiresIn <= 0 {
		expiresIn = time.Minute
	}

	return token, time.Now().Add(expiresIn), nil
}

func (c *Client) registryCredentials(ctx context.Context, registry string) (*Credentials, error) {
	if c.credentials == nil {
		return nil, nil
	}

	return c.cache.credentialsFor(registry, func() (*Credentials, error) {
		credentials, err := c.credentials.GetRegistryCredentials(ctx, registry)

		if err != nil {
			return nil, fmt.Errorf("loading registry credentials: %w", err)
		}

		return credentials, nil
	})
}

func (c *Client) registryOptions(ctx context.Context, registry string) (*RegistryOptions, error) {
	if c.options == nil {
		return &RegistryOptions{}, nil
	}

	return c.cache.optionsFor(registry, func() (*RegistryOptions, error) {
		options, err := c.options.GetRegistryOptions(ctx, registry)

		if err != nil {
			return nil, fmt.Errorf("loading registry options: %w", err)
		}

		if options == nil {
			options = &RegistryOptions{}
		}

		return options, nil
	})
}

func (c *Client) do(req *http.Request, registry string, options *RegistryOptions) (*http.Response, error) {
	client := c.httpClient

	if options.AllowSelfSignedCerts {
		client = c.insecureClientFor(registry)
	}

	for attempt := 0; ; attempt++ {
		response, err := client.Do(req)

		if err != nil {
			return nil, err
		}

		if attempt >= maxRetryAttempts || !isRetryableStatus(response.StatusCode) {
			return response, nil
		}

		delay := retryDelay(attempt, response)
		drainAndClose(response)

		select {
		case <-time.After(delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

func retryDelay(attempt int, response *http.Response) time.Duration {
	base := time.Duration(1<<attempt) * time.Second

	if wait, ok := parseRetryAfter(response.Header.Get("Retry-After")); ok {
		base = min(wait, maxRetryAfterDelay)
	}

	return base + rand.N(base/4+1)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)

	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, false
		}

		return time.Duration(seconds) * time.Second, true
	}

	if when, err := http.ParseTime(value); err == nil {
		if wait := time.Until(when); wait > 0 {
			return wait, true
		}
	}

	return 0, false
}

func (c *Client) insecureClientFor(registry string) *http.Client {
	return c.cache.insecureClient(registry, func() *http.Client {
		baseTransport := c.httpClient.Transport
		if baseTransport == nil {
			baseTransport = http.DefaultTransport
		}

		transport, ok := baseTransport.(*http.Transport)

		if !ok {
			return c.httpClient
		}

		transport = transport.Clone()

		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		} else {
			transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		}

		transport.TLSClientConfig.InsecureSkipVerify = true

		return &http.Client{
			Transport:     transport,
			CheckRedirect: c.httpClient.CheckRedirect,
			Jar:           c.httpClient.Jar,
			Timeout:       c.httpClient.Timeout,
		}
	})
}

func credentialIdentity(credentials *Credentials) string {
	if credentials == nil {
		return "anonymous"
	}

	fingerprint := sha256.Sum256([]byte(credentials.Username + "\x00" + credentials.Credential))

	return credentials.Username + "\x00" + hex.EncodeToString(fingerprint[:])
}

func authenticationChallenge(challenge string) (scheme, value string) {
	challenge = strings.TrimSpace(challenge)

	if challenge == "" {
		return "", ""
	}

	scheme, value, _ = strings.Cut(challenge, " ")

	return scheme, strings.TrimSpace(value)
}

func bearerParams(value string) map[string]string {
	params := make(map[string]string)

	for _, param := range paramRe.FindAllStringSubmatch(value, -1) {
		params[param[1]] = param[2]
	}

	return params
}

func (c *Client) tokenCacheKey(registry, repository string, credentials *Credentials) string {
	return registry + "\x00" + repository + "\x00" + credentialIdentity(credentials)
}

func RegistryAPIHost(registry string) string {
	if registry == "" || registry == "docker.io" {
		return "registry-1.docker.io"
	}

	return registry
}

func requestForRegistry(req *http.Request, options *RegistryOptions) *http.Request {
	request := req.Clone(req.Context())
	request.URL = cloneURL(req.URL)
	request.URL.Scheme = "https"

	if options.Insecure {
		request.URL.Scheme = "http"
	}

	return request
}

func cloneURL(value *url.URL) *url.URL {
	clone := *value
	return &clone
}

func RepositoryPath(registry, repository string) string {
	if registry == "" || registry == "docker.io" || registry == "registry-1.docker.io" {
		if rest, ok := strings.CutPrefix(repository, "_/"); ok {
			return "library/" + rest
		}

		if !strings.Contains(repository, "/") {
			return "library/" + repository
		}
	}

	return repository
}

func nextPageURL(base *url.URL, link string) string {
	for _, value := range splitLinkValues(link) {
		parts := strings.Split(value, ";")

		if len(parts) < 2 {
			continue
		}

		isNext := false

		for _, parameter := range parts[1:] {
			key, value, found := strings.Cut(strings.TrimSpace(parameter), "=")

			if found && strings.EqualFold(key, "rel") && strings.EqualFold(strings.Trim(value, `"`), "next") {
				isNext = true
				break
			}
		}

		if !isNext {
			continue
		}

		target := strings.Trim(strings.TrimSpace(parts[0]), "<>")

		next, err := base.Parse(target)

		if err == nil {
			return next.String()
		}
	}

	return ""
}

func splitLinkValues(link string) []string {
	values := make([]string, 0, 1)
	start := 0
	inAngle, inQuote := false, false

	for index, character := range link {
		switch character {
		case '<':
			if !inQuote {
				inAngle = true
			}

		case '>':
			if !inQuote {
				inAngle = false
			}

		case '"':
			if !inAngle {
				inQuote = !inQuote
			}

		case ',':
			if !inAngle && !inQuote {
				values = append(values, strings.TrimSpace(link[start:index]))

				start = index + 1
			}
		}
	}

	values = append(values, strings.TrimSpace(link[start:]))

	return values
}
