package oci

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
)

const registryDataCacheTTL = 30 * time.Second
const tokenCacheMaxEntries = 512
const tagsListPageSize = 1000

var (
	paramRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

	excludedTagPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^pr-\d+`),
		regexp.MustCompile(`^commit-[0-9a-f]+`),
	}
)

func isExcludedTag(tag string) bool {
	for _, re := range excludedTagPatterns {
		if re.MatchString(tag) {
			return true
		}
	}

	return false
}

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
	httpClient       *http.Client
	credentials      credentialProvider
	options          registryOptionsProvider
	tokens           *expirable.LRU[string, cachedToken]
	tokenFetch       singleflight.Group
	insecureClients  *expirable.LRU[string, *http.Client]
	credentialsCache *expirable.LRU[string, *Credentials]
	credentialsFetch singleflight.Group
	optionsCache     *expirable.LRU[string, *RegistryOptions]
	optionsFetch     singleflight.Group
}

type cachedToken struct {
	value     string
	expiresAt time.Time
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
		httpClient:       httpClient,
		credentials:      credentials,
		options:          options,
		tokens:           expirable.NewLRU[string, cachedToken](tokenCacheMaxEntries, nil, 0),
		insecureClients:  expirable.NewLRU[string, *http.Client](0, nil, registryDataCacheTTL),
		credentialsCache: expirable.NewLRU[string, *Credentials](0, nil, registryDataCacheTTL),
		optionsCache:     expirable.NewLRU[string, *RegistryOptions](0, nil, registryDataCacheTTL),
	}
}

func (c *Client) ListTags(ctx context.Context, registry, repository string) ([]string, error) {
	host := RegistryAPIHost(registry)
	path := RepositoryPath(registry, repository)

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
			registryErr := &Error{
				StatusCode: response.StatusCode,
				Message:    fmt.Sprintf("registry returned %s", response.Status),
			}

			response.Body.Close()

			return nil, registryErr
		}

		var page struct {
			Tags []string `json:"tags"`
		}

		decodeErr := json.NewDecoder(response.Body).Decode(&page)
		next := nextPageURL(response.Request.URL, response.Header.Get("Link"))

		response.Body.Close()

		if decodeErr != nil {
			return nil, fmt.Errorf("decoding tag list from %s: %w", response.Request.URL.Host, decodeErr)
		}

		for _, tag := range page.Tags {
			if isExcludedTag(tag) {
				continue
			}

			tags = append(tags, tag)
		}

		endpoint = next
	}

	return tags, nil
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

		if token, ok := c.cachedToken(key); ok {
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
		c.tokens.Remove(preemptiveKey)
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

	if token, ok := c.cachedToken(cacheKey); ok {
		return token, nil
	}

	tokenValue, err, _ := c.tokenFetch.Do(cacheKey, func() (any, error) {
		if token, ok := c.cachedToken(cacheKey); ok {
			return token, nil
		}

		return c.requestBearerToken(ctx, cacheKey, realm, registry, params, credentials, options)
	})

	if err != nil {
		return "", err
	}

	return tokenValue.(string), nil
}

func (c *Client) requestBearerToken(ctx context.Context, cacheKey, realm, registry string, params map[string]string, credentials *Credentials, options *RegistryOptions) (string, error) {
	tokenURL, err := url.Parse(realm)

	if err != nil {
		return "", fmt.Errorf("parsing token realm: %w", err)
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
		return "", fmt.Errorf("creating token request: %w", err)
	}

	if credentials != nil {
		req.SetBasicAuth(credentials.Username, credentials.Credential)
	}

	response, err := c.do(requestForRegistry(req, options), registry, options)

	if err != nil {
		return "", fmt.Errorf("requesting registry token: %w", err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %s", response.Status)
	}

	var tokenResponse struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}

	if err := json.NewDecoder(response.Body).Decode(&tokenResponse); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	token := tokenResponse.Token

	if token == "" {
		token = tokenResponse.AccessToken
	}

	if token == "" {
		return "", fmt.Errorf("token response did not contain a token")
	}

	expiresIn := time.Duration(tokenResponse.ExpiresIn) * time.Second

	if expiresIn <= 0 {
		expiresIn = time.Minute
	}

	c.cacheToken(cacheKey, token, time.Now().Add(expiresIn))

	return token, nil
}

func (c *Client) registryCredentials(ctx context.Context, registry string) (*Credentials, error) {
	if c.credentials == nil {
		return nil, nil
	}

	if credentials, ok := c.credentialsCache.Get(registry); ok {
		return credentials, nil
	}

	result, err, _ := c.credentialsFetch.Do(registry, func() (any, error) {
		if credentials, ok := c.credentialsCache.Get(registry); ok {
			return credentials, nil
		}

		credentials, err := c.credentials.GetRegistryCredentials(ctx, registry)

		if err != nil {
			return nil, fmt.Errorf("loading registry credentials: %w", err)
		}

		c.credentialsCache.Add(registry, credentials)

		return credentials, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*Credentials), nil
}

func (c *Client) registryOptions(ctx context.Context, registry string) (*RegistryOptions, error) {
	if c.options == nil {
		return &RegistryOptions{}, nil
	}

	if options, ok := c.optionsCache.Get(registry); ok {
		return options, nil
	}

	result, err, _ := c.optionsFetch.Do(registry, func() (any, error) {
		if options, ok := c.optionsCache.Get(registry); ok {
			return options, nil
		}

		options, err := c.options.GetRegistryOptions(ctx, registry)
		if err != nil {
			return nil, fmt.Errorf("loading registry options: %w", err)
		}

		if options == nil {
			options = &RegistryOptions{}
		}

		c.optionsCache.Add(registry, options)

		return options, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*RegistryOptions), nil
}

func (c *Client) do(req *http.Request, registry string, options *RegistryOptions) (*http.Response, error) {
	client := c.httpClient

	if options.AllowSelfSignedCerts {
		client = c.insecureClientFor(registry)
	}

	return client.Do(req)
}

func (c *Client) insecureClientFor(registry string) *http.Client {
	if client, ok := c.insecureClients.Get(registry); ok {
		return client
	}

	baseTransport := c.httpClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	transport, ok := baseTransport.(*http.Transport)

	if !ok {
		c.insecureClients.Add(registry, c.httpClient)
		return c.httpClient
	}

	transport = transport.Clone()

	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}

	transport.TLSClientConfig.InsecureSkipVerify = true

	client := &http.Client{
		Transport:     transport,
		CheckRedirect: c.httpClient.CheckRedirect,
		Jar:           c.httpClient.Jar,
		Timeout:       c.httpClient.Timeout,
	}

	c.insecureClients.Add(registry, client)

	return client
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

func (c *Client) cachedToken(key string) (string, bool) {
	token, ok := c.tokens.Get(key)

	if !ok || !token.expiresAt.After(time.Now().Add(5*time.Second)) {
		c.tokens.Remove(key)

		return "", false
	}

	return token.value, true
}

func (c *Client) cacheToken(key, value string, expiresAt time.Time) {
	c.tokens.Add(key, cachedToken{
		value:     value,
		expiresAt: expiresAt,
	})
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
