package config

import (
	"slices"
	"strings"
	"testing"
)

func TestInternalRegistryDefaults(t *testing.T) {
	cfg, err := Load("")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.InternalRegistry.Enabled {
		t.Errorf("internal_registry.enabled = true by default, want false")
	}

	if cfg.InternalRegistry.Address != "registry:5000" {
		t.Errorf("internal_registry.address = %q, want registry:5000", cfg.InternalRegistry.Address)
	}

	if cfg.InternalRegistry.TLS {
		t.Errorf("internal_registry.tls = true by default, want false")
	}
}

func TestInternalRegistryFromEnv(t *testing.T) {
	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_ENABLED", "true")
	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_ADDRESS", " images:5000 ")
	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_TLS", "true")

	cfg, err := Load("")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.InternalRegistry.Enabled || cfg.InternalRegistry.Address != "images:5000" || !cfg.InternalRegistry.TLS {
		t.Errorf("internal_registry = %+v, want enabled at images:5000 with TLS", cfg.InternalRegistry)
	}
}

func TestInternalRegistryWarnsWithoutProxyAuth(t *testing.T) {
	warned := func(cfg *Config) bool {
		return slices.ContainsFunc(cfg.DerivedWarnings, func(w string) bool {
			return strings.Contains(w, "accepts pushes from anyone")
		})
	}

	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_ENABLED", "true")

	cfg, err := Load("")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !warned(cfg) {
		t.Errorf("no warning with proxy_auth off: %v", cfg.DerivedWarnings)
	}

	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_PUSH_ALLOWED_IPS", "10.0.0.5, 10.0.1.0/24")

	if cfg, err = Load(""); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if warned(cfg) || !slices.Equal(cfg.InternalRegistry.PushAllowedIPs, []string{"10.0.0.5", "10.0.1.0/24"}) {
		t.Errorf("with push_allowed_ips %v: warnings %v; want no warning", cfg.InternalRegistry.PushAllowedIPs, cfg.DerivedWarnings)
	}

	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_PUSH_ALLOWED_IPS", "")
	t.Setenv("DOCKVMAP_PROXY_AUTH_ENABLED", "true")

	if cfg, err = Load(""); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if warned(cfg) {
		t.Errorf("warning with proxy_auth on: %v", cfg.DerivedWarnings)
	}
}

var testToken = "dvmr1-" + strings.Repeat("ab", 32) + "-" + strings.Repeat("cd", 32)

func TestReplicationValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"primary and follower", map[string]string{"DOCKVMAP_REPLICATION_ENABLED": "true", "DOCKVMAP_FOLLOWER_ENABLED": "true", "DOCKVMAP_FOLLOWER_PRIMARY": "10.0.0.11:9003", "DOCKVMAP_FOLLOWER_TOKEN": "t"}, "can't also serve replication"},
		{"bad listen", map[string]string{"DOCKVMAP_REPLICATION_ENABLED": "true", "DOCKVMAP_REPLICATION_LISTEN": "9003"}, "replication.listen"},
		{"follower without primary", map[string]string{"DOCKVMAP_FOLLOWER_ENABLED": "true", "DOCKVMAP_FOLLOWER_TOKEN": "t"}, "follower.primary"},
		{"follower without token", map[string]string{"DOCKVMAP_FOLLOWER_ENABLED": "true", "DOCKVMAP_FOLLOWER_PRIMARY": "10.0.0.11:9003"}, "follower.token"},
		{"follower with a malformed token", map[string]string{"DOCKVMAP_FOLLOWER_ENABLED": "true", "DOCKVMAP_FOLLOWER_PRIMARY": "10.0.0.11:9003", "DOCKVMAP_FOLLOWER_TOKEN": "abc"}, "isn't a replication token"},
		{"negative reconcile", map[string]string{"DOCKVMAP_FOLLOWER_ENABLED": "true", "DOCKVMAP_FOLLOWER_PRIMARY": "10.0.0.11:9003", "DOCKVMAP_FOLLOWER_TOKEN": testToken, "DOCKVMAP_FOLLOWER_RECONCILE_INTERVAL": "-1h"}, "follower.reconcile_interval"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := Load("")

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load() error = %v, want one mentioning %q", err, tt.want)
			}
		})
	}
}

func TestFollowerReconcileCanBeTurnedOff(t *testing.T) {
	t.Setenv("DOCKVMAP_FOLLOWER_ENABLED", "true")
	t.Setenv("DOCKVMAP_FOLLOWER_PRIMARY", "10.0.0.11:9003")
	t.Setenv("DOCKVMAP_FOLLOWER_TOKEN", testToken)
	t.Setenv("DOCKVMAP_FOLLOWER_RECONCILE_INTERVAL", "0")

	cfg, err := Load("")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Follower.ReconcileIntervalDuration() != 0 {
		t.Errorf("reconcile interval = %v, want 0 (off)", cfg.Follower.ReconcileIntervalDuration())
	}

	if cfg.Replication.Listen != ":9003" {
		t.Errorf("replication.listen default = %q, want :9003", cfg.Replication.Listen)
	}
}

func TestFollowerDoesNotWarnAboutOpenPushes(t *testing.T) {
	t.Setenv("DOCKVMAP_INTERNAL_REGISTRY_ENABLED", "true")
	t.Setenv("DOCKVMAP_FOLLOWER_ENABLED", "true")
	t.Setenv("DOCKVMAP_FOLLOWER_PRIMARY", "10.0.0.11:9003")
	t.Setenv("DOCKVMAP_FOLLOWER_TOKEN", testToken)

	cfg, err := Load("")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, warning := range cfg.DerivedWarnings {
		if strings.Contains(warning, "accepts pushes from anyone") {
			t.Errorf("follower warns %q, want no push warning since it refuses every push", warning)
		}
	}
}
