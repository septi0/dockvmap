package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

var virtualTagRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

type Config struct {
	VirtualTag              string               `yaml:"virtual_tag" env:"DOCKVMAP_VIRTUAL_TAG" default:"current"`
	TagsCheckInterval       string               `yaml:"tags_check_interval" env:"DOCKVMAP_TAGS_CHECK_INTERVAL" default:"24h"`
	TagDiscoveryTTL         string               `yaml:"tag_discovery_ttl" env:"DOCKVMAP_TAG_DISCOVERY_TTL" default:"1h"`
	ProxyServerListen       string               `yaml:"proxy_server_listen" env:"DOCKVMAP_PROXY_SERVER_LISTEN" default:":5000"`
	ProxyPublicHost         string               `yaml:"proxy_public_host" env:"DOCKVMAP_PROXY_PUBLIC_HOST"`
	WebServerListen         string               `yaml:"web_server_listen" env:"DOCKVMAP_WEB_SERVER_LISTEN" default:":8080"`
	LogsPath                string               `yaml:"logs_path" env:"DOCKVMAP_LOGS_PATH"`
	TagFiltersPath          string               `yaml:"tag_filters_path" env:"DOCKVMAP_TAG_FILTERS_PATH"`
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

	DerivedWarnings []string `yaml:"-"`
}

type BlobCacheConfig struct {
	Enabled         *bool  `yaml:"enabled" env:"DOCKVMAP_BLOB_CACHE_ENABLED" default:"true"`
	Lifetime        string `yaml:"lifetime" env:"DOCKVMAP_BLOB_CACHE_LIFETIME" default:"24h"`
	CleanupInterval string `yaml:"cleanup_interval" env:"DOCKVMAP_BLOB_CACHE_CLEANUP_INTERVAL" default:"1h"`
	MaxSize         string `yaml:"max_size" env:"DOCKVMAP_BLOB_CACHE_MAX_SIZE" default:"10GB"`
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
	if c.SMTP.Enabled && c.SMTP.Host == "" {
		c.SMTP.Enabled = false
		c.warn("smtp.enabled is set but host is blank; SMTP notifications have been disabled")
	}

	if c.SecureCookies == nil {
		secure := c.TLS.Enabled
		c.SecureCookies = &secure
	}
}

func (c *Config) warn(message string) {
	c.DerivedWarnings = append(c.DerivedWarnings, message)
}

func positiveDuration(name, value string) error {
	d, err := time.ParseDuration(value)

	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", name, value, err)
	}

	if d <= 0 {
		return fmt.Errorf("invalid %s %q: must be a positive duration", name, value)
	}

	return nil
}

func (c *Config) validate() error {
	durations := []struct{ name, value string }{
		{"session_lifetime", c.SessionLifetime},
		{"login_rate_limit.window", c.LoginRateLimit.Window},
		{"tag_discovery_ttl", c.TagDiscoveryTTL},
		{"tags_check_interval", c.TagsCheckInterval},
		{"blob_cache.lifetime", c.BlobCache.Lifetime},
		{"blob_cache.cleanup_interval", c.BlobCache.CleanupInterval},
	}

	for _, d := range durations {
		if err := positiveDuration(d.name, d.value); err != nil {
			return err
		}
	}

	if c.LoginRateLimit.MaxAttempts <= 0 {
		return fmt.Errorf("invalid login_rate_limit.max_attempts %d: must be positive", c.LoginRateLimit.MaxAttempts)
	}

	if _, err := parseBytes(c.BlobCache.MaxSize); err != nil {
		return fmt.Errorf("blob_cache.max_size: %w", err)
	}

	if !virtualTagRE.MatchString(c.VirtualTag) {
		return fmt.Errorf("invalid virtual_tag %q: must be a valid image tag", c.VirtualTag)
	}

	if c.TLS.Enabled && (c.TLS.CertFile == "" || c.TLS.KeyFile == "") {
		return fmt.Errorf("tls.enabled is set but cert_file or key_file is blank")
	}

	if c.SMTP.Enabled {
		if c.SMTP.Port < 1 || c.SMTP.Port > 65535 {
			return fmt.Errorf("invalid smtp.port %d: must be between 1 and 65535", c.SMTP.Port)
		}

		if c.SMTP.From == "" {
			return fmt.Errorf("smtp.enabled is set but smtp.from is blank")
		}
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

func (c *Config) BlobCacheMaxSizeBytes() int64 {
	n, _ := parseBytes(c.BlobCache.MaxSize)
	return n
}

func Load(path string) (*Config, error) {
	var cfg Config

	if path != "" {
		data, err := os.ReadFile(path)

		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}

		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)

		if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
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
