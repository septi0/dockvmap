package config

import (
	"fmt"
	"os"
	"time"

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
	SecureCookies           *bool                `yaml:"secure_cookies"`
	TrustedProxies          []string             `yaml:"trusted_proxies"`
	Webhooks                []string             `yaml:"webhooks"`
	BlobCache               BlobCacheConfig      `yaml:"blob_cache"`
	SMTP                    SMTPConfig           `yaml:"smtp"`
	TLS                     TLSConfig            `yaml:"tls"`
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

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

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

	if c.TLS.Enabled && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		c.TLS.Enabled = false
	}

	if c.SMTP.Enabled && c.SMTP.Host == "" {
		c.SMTP.Enabled = false
	}

	if c.SecureCookies == nil {
		secure := c.TLS.Enabled
		c.SecureCookies = &secure
	}
}

func (c *Config) validate() error {
	if d, err := time.ParseDuration(c.SessionLifetime); err != nil || d <= 0 {
		return fmt.Errorf("invalid session_lifetime %q: %w", c.SessionLifetime, err)
	}

	if d, err := time.ParseDuration(c.LoginRateLimit.Window); err != nil || d <= 0 {
		return fmt.Errorf("invalid login_rate_limit.window %q: %w", c.LoginRateLimit.Window, err)
	}

	if c.LoginRateLimit.MaxAttempts <= 0 {
		return fmt.Errorf("invalid login_rate_limit.max_attempts %d: must be positive", c.LoginRateLimit.MaxAttempts)
	}

	return nil
}

func (c *Config) SessionLifetimeDuration() time.Duration {
	d, _ := time.ParseDuration(c.SessionLifetime)
	return d
}

func (l LoginRateLimitConfig) WindowDuration() time.Duration {
	d, _ := time.ParseDuration(l.Window)
	return d
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

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
