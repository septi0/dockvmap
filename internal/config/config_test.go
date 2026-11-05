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

	t.Setenv("DOCKVMAP_PROXY_AUTH_ENABLED", "true")

	if cfg, err = Load(""); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if warned(cfg) {
		t.Errorf("warning with proxy_auth on: %v", cfg.DerivedWarnings)
	}
}
