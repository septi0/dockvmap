package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	VirtualTag              string               `yaml:"virtual_tag" env:"DOCKVMAP_VIRTUAL_TAG" default:"current"`
	TagsCheckInterval       string               `yaml:"tags_check_interval" env:"DOCKVMAP_TAGS_CHECK_INTERVAL" default:"24h"`
	TagDiscoveryTTL         string               `yaml:"tag_discovery_ttl" env:"DOCKVMAP_TAG_DISCOVERY_TTL" default:"1h"`
	ProxyServerListen       string               `yaml:"proxy_server_listen" env:"DOCKVMAP_PROXY_SERVER_LISTEN" default:":5000"`
	ProxyPublicHost         string               `yaml:"proxy_public_host" env:"DOCKVMAP_PROXY_PUBLIC_HOST"`
	WebServerListen         string               `yaml:"web_server_listen" env:"DOCKVMAP_WEB_SERVER_LISTEN" default:":8080"`
	DataPath                string               `yaml:"data_path" env:"DOCKVMAP_DATA_PATH" default:"./data"`
	LogsPath                string               `yaml:"logs_path" env:"DOCKVMAP_LOGS_PATH"`
	CredentialEncryptionKey string               `yaml:"credential_encryption_key" env:"DOCKVMAP_CREDENTIAL_ENCRYPTION_KEY"`
	SessionLifetime         string               `yaml:"session_lifetime" env:"DOCKVMAP_SESSION_LIFETIME" default:"168h"`
	SecureCookies           *bool                `yaml:"secure_cookies" env:"DOCKVMAP_SECURE_COOKIES"`
	TrustedProxies          []string             `yaml:"trusted_proxies" env:"DOCKVMAP_TRUSTED_PROXIES"`
	Webhooks                []string             `yaml:"webhooks" env:"DOCKVMAP_WEBHOOKS"`
	BlobCache               BlobCacheConfig      `yaml:"blob_cache"`
	SMTP                    SMTPConfig           `yaml:"smtp"`
	TLS                     TLSConfig            `yaml:"tls"`
	ProxyAuth               ProxyAuthConfig      `yaml:"proxy_auth"`
	LoginRateLimit          LoginRateLimitConfig `yaml:"login_rate_limit"`
}

type BlobCacheConfig struct {
	Enabled         *bool  `yaml:"enabled" env:"DOCKVMAP_BLOB_CACHE_ENABLED" default:"true"`
	Path            string `yaml:"path" env:"DOCKVMAP_BLOB_CACHE_PATH"`
	Lifetime        string `yaml:"lifetime" env:"DOCKVMAP_BLOB_CACHE_LIFETIME" default:"24h"`
	CleanupInterval string `yaml:"cleanup_interval" env:"DOCKVMAP_BLOB_CACHE_CLEANUP_INTERVAL" default:"1h"`
}

type SMTPConfig struct {
	Enabled  bool   `yaml:"enabled" env:"DOCKVMAP_SMTP_ENABLED"`
	Host     string `yaml:"host" env:"DOCKVMAP_SMTP_HOST"`
	Port     int    `yaml:"port" env:"DOCKVMAP_SMTP_PORT" default:"587"`
	Username string `yaml:"username" env:"DOCKVMAP_SMTP_USERNAME"`
	Password string `yaml:"password" env:"DOCKVMAP_SMTP_PASSWORD"`
	From     string `yaml:"from" env:"DOCKVMAP_SMTP_FROM"`
	TLS      *bool  `yaml:"tls" env:"DOCKVMAP_SMTP_TLS" default:"true"`
}

type ProxyAuthConfig struct {
	Enabled bool `yaml:"enabled" env:"DOCKVMAP_PROXY_AUTH_ENABLED"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled" env:"DOCKVMAP_TLS_ENABLED"`
	CertFile string `yaml:"cert_file" env:"DOCKVMAP_TLS_CERT_FILE"`
	KeyFile  string `yaml:"key_file" env:"DOCKVMAP_TLS_KEY_FILE"`
}

type LoginRateLimitConfig struct {
	Enabled     *bool    `yaml:"enabled" env:"DOCKVMAP_LOGIN_RATE_LIMIT_ENABLED" default:"true"`
	MaxAttempts int      `yaml:"max_attempts" env:"DOCKVMAP_LOGIN_RATE_LIMIT_MAX_ATTEMPTS" default:"5"`
	Window      string   `yaml:"window" env:"DOCKVMAP_LOGIN_RATE_LIMIT_WINDOW" default:"15m"`
	BypassIPs   []string `yaml:"bypass_ips" env:"DOCKVMAP_LOGIN_RATE_LIMIT_BYPASS_IPS"`
}

func (c *Config) applyDerivedDefaults() {
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

	if c.BlobCache.Path == "" {
		c.BlobCache.Path = filepath.Join(c.DataPath, "cache")
	}
}

func (c *Config) validate() error {
	if d, err := time.ParseDuration(c.SessionLifetime); err != nil || d <= 0 {
		return fmt.Errorf("invalid session_lifetime %q: %w", c.SessionLifetime, err)
	}

	if d, err := time.ParseDuration(c.LoginRateLimit.Window); err != nil || d <= 0 {
		return fmt.Errorf("invalid login_rate_limit.window %q: %w", c.LoginRateLimit.Window, err)
	}

	if d, err := time.ParseDuration(c.TagDiscoveryTTL); err != nil || d <= 0 {
		return fmt.Errorf("invalid tag_discovery_ttl %q: %w", c.TagDiscoveryTTL, err)
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

func (c *Config) TagDiscoveryTTLDuration() time.Duration {
	d, _ := time.ParseDuration(c.TagDiscoveryTTL)
	return d
}

func Load(path string) (*Config, error) {
	var cfg Config

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}

	if err := applyDefaults(&cfg); err != nil {
		return nil, err
	}

	cfg.applyDerivedDefaults()

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
