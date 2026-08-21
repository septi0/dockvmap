package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	VirtualTag              string               `yaml:"virtual_tag"`
	TagsCheckInterval       string               `yaml:"tags_check_interval"`
	ProxyServerListen       string               `yaml:"proxy_server_listen"`
	WebServerListen         string               `yaml:"web_server_listen"`
	DataPath                string               `yaml:"data_path"`
	LogsPath                string               `yaml:"logs_path"`
	CredentialEncryptionKey string               `yaml:"credential_encryption_key"`
	SessionLifetime         string               `yaml:"session_lifetime"`
	SecureCookies           bool                 `yaml:"secure_cookies"`
	TrustedProxies          []string             `yaml:"trusted_proxies"`
	Webhooks                []string             `yaml:"webhooks"`
	BlobCache               BlobCacheConfig      `yaml:"blob_cache"`
	SMTP                    SMTPConfig           `yaml:"smtp"`
	ProxyAuth               ProxyAuthConfig      `yaml:"proxy_auth"`
	LoginRateLimit          LoginRateLimitConfig `yaml:"login_rate_limit"`
}

type BlobCacheConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Path            string `yaml:"path"`
	Lifetime        string `yaml:"lifetime"`
	CleanupInterval string `yaml:"cleanup_interval"`
}

type SMTPConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	TLS      bool   `yaml:"tls"`
}

type ProxyAuthConfig struct {
	Enabled bool `yaml:"enabled"`
}

// Enabled is a pointer so an absent login_rate_limit block (e.g. an existing
// config.yaml predating this feature) defaults to protected, not to
// unprotected - unlike the other opt-in config blocks, turning this on
// doesn't change behavior for anyone entering a real password.
type LoginRateLimitConfig struct {
	Enabled     *bool    `yaml:"enabled"`
	MaxAttempts int      `yaml:"max_attempts"`
	Window      string   `yaml:"window"`
	BypassIPs   []string `yaml:"bypass_ips"`
}

func (c *Config) setDefaults() {
	if c.VirtualTag == "" {
		c.VirtualTag = "current"
	}

	if c.TagsCheckInterval == "" {
		c.TagsCheckInterval = "24h"
	}

	if c.SessionLifetime == "" {
		c.SessionLifetime = "168h"
	}

	if c.WebServerListen == "" {
		c.WebServerListen = ":8080"
	}

	if c.ProxyServerListen == "" {
		c.ProxyServerListen = ":5000"
	}

	if c.DataPath == "" {
		c.DataPath = "./data"
	}

	if c.BlobCache.Path == "" {
		c.BlobCache.Path = "./cache"
	}

	if c.BlobCache.Lifetime == "" {
		c.BlobCache.Lifetime = "24h"
	}

	if c.BlobCache.CleanupInterval == "" {
		c.BlobCache.CleanupInterval = "1h"
	}

	if c.LoginRateLimit.Enabled == nil {
		enabled := true
		c.LoginRateLimit.Enabled = &enabled
	}

	if c.LoginRateLimit.MaxAttempts == 0 {
		c.LoginRateLimit.MaxAttempts = 5
	}

	if c.LoginRateLimit.Window == "" {
		c.LoginRateLimit.Window = "15m"
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.setDefaults()

	return &cfg, nil
}
