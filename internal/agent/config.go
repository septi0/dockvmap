package agent

import (
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultDockerHost = "unix:///var/run/docker.sock"

type Config struct {
	URL        *url.URL
	Token      string
	RootCAs    *x509.CertPool
	ProxyOnly  bool
	DockerHost string
	LogLevel   slog.Level
}

func LoadConfig(getenv func(string) string) (Config, error) {
	var cfg Config

	rawURL := strings.TrimSpace(getenv("DOCKVMAP_URL"))

	if rawURL == "" {
		return cfg, errors.New("DOCKVMAP_URL is required, e.g. https://dockvmap.example.com")
	}

	parsed, err := url.Parse(rawURL)

	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return cfg, fmt.Errorf("DOCKVMAP_URL must be an http:// or https:// address, got %q", rawURL)
	}

	cfg.URL = parsed

	if cfg.Token, err = loadToken(getenv); err != nil {
		return cfg, err
	}

	if caFile := strings.TrimSpace(getenv("DOCKVMAP_CA_FILE")); caFile != "" {
		if cfg.RootCAs, err = loadCAFile(caFile); err != nil {
			return cfg, err
		}
	}

	if raw := strings.TrimSpace(getenv("DOCKVMAP_AGENT_PROXY_ONLY")); raw != "" {
		if cfg.ProxyOnly, err = strconv.ParseBool(raw); err != nil {
			return cfg, fmt.Errorf("DOCKVMAP_AGENT_PROXY_ONLY must be true or false, got %q", raw)
		}
	}

	cfg.DockerHost = strings.TrimSpace(getenv("DOCKER_HOST"))

	if cfg.DockerHost == "" {
		cfg.DockerHost = defaultDockerHost
	}

	if raw := strings.TrimSpace(getenv("DOCKVMAP_LOG_LEVEL")); raw != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(raw)); err != nil {
			return cfg, fmt.Errorf("DOCKVMAP_LOG_LEVEL must be debug, info, warn or error, got %q", raw)
		}
	}

	return cfg, nil
}

func loadToken(getenv func(string) string) (string, error) {
	token := strings.TrimSpace(getenv("DOCKVMAP_AGENT_TOKEN"))
	tokenFile := strings.TrimSpace(getenv("DOCKVMAP_AGENT_TOKEN_FILE"))

	switch {
	case token != "" && tokenFile != "":
		return "", errors.New("set either DOCKVMAP_AGENT_TOKEN or DOCKVMAP_AGENT_TOKEN_FILE, not both")

	case tokenFile != "":
		content, err := os.ReadFile(tokenFile)

		if err != nil {
			return "", fmt.Errorf("reading DOCKVMAP_AGENT_TOKEN_FILE: %w", err)
		}

		token = strings.TrimSpace(string(content))
	}

	if token == "" {
		return "", errors.New("DOCKVMAP_AGENT_TOKEN is required; copy it from the Hosts page in dockvmap")
	}

	return token, nil
}

func loadCAFile(path string) (*x509.CertPool, error) {
	content, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("reading DOCKVMAP_CA_FILE: %w", err)
	}

	pool, err := x509.SystemCertPool()

	if err != nil {
		pool = x509.NewCertPool()
	}

	if !pool.AppendCertsFromPEM(content) {
		return nil, fmt.Errorf("DOCKVMAP_CA_FILE %q contains no PEM certificates", path)
	}

	return pool, nil
}
