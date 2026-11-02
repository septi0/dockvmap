package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	badCA := filepath.Join(dir, "bad-ca.pem")

	if err := os.WriteFile(tokenFile, []byte("secret-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(badCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	base := map[string]string{"DOCKVMAP_URL": "https://dockvmap.example.com", "DOCKVMAP_AGENT_TOKEN": "secret"}

	with := func(changes map[string]string) map[string]string {
		values := map[string]string{}

		for k, v := range base {
			values[k] = v
		}

		for k, v := range changes {
			values[k] = v
		}

		return values
	}

	cfg, err := LoadConfig(env(base))

	if err != nil || cfg.Token != "secret" || cfg.DockerHost != defaultDockerHost || cfg.ProxyOnly || cfg.LogLevel != slog.LevelInfo || cfg.RootCAs != nil {
		t.Fatalf("defaults = %+v, err = %v", cfg, err)
	}

	cfg, err = LoadConfig(env(with(map[string]string{
		"DOCKVMAP_AGENT_TOKEN":      "",
		"DOCKVMAP_AGENT_TOKEN_FILE": tokenFile,
		"DOCKVMAP_AGENT_PROXY_ONLY": "true",
		"DOCKER_HOST":               "tcp://socket-proxy:2375",
		"DOCKVMAP_LOG_LEVEL":        "debug",
	})))

	if err != nil || cfg.Token != "secret-from-file" || !cfg.ProxyOnly || cfg.DockerHost != "tcp://socket-proxy:2375" || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("custom = %+v, err = %v", cfg, err)
	}

	bad := map[string]map[string]string{
		"DOCKVMAP_URL is required":  {"DOCKVMAP_URL": ""},
		"http:// or https://":       {"DOCKVMAP_URL": "ftp://dockvmap.example.com"},
		"either":                    {"DOCKVMAP_AGENT_TOKEN_FILE": tokenFile},
		"copy it from the Hosts":    {"DOCKVMAP_AGENT_TOKEN": "  "},
		"reading DOCKVMAP_AGENT":    {"DOCKVMAP_AGENT_TOKEN": "", "DOCKVMAP_AGENT_TOKEN_FILE": filepath.Join(dir, "missing")},
		"no PEM certificates":       {"DOCKVMAP_CA_FILE": badCA},
		"must be true or false":     {"DOCKVMAP_AGENT_PROXY_ONLY": "maybe"},
		"must be debug, info, warn": {"DOCKVMAP_LOG_LEVEL": "loud"},
	}

	for want, changes := range bad {
		if _, err := LoadConfig(env(with(changes))); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: err = %v, want it to mention %q", changes, err, want)
		}
	}
}
