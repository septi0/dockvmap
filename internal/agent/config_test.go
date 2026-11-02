package agent

import (
	"log/slog"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(env(nil))

	if err != nil || cfg.Listen != ":9002" || cfg.DockerHost != defaultDockerHost || cfg.ProxyOnly || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("defaults = %+v, err = %v", cfg, err)
	}

	cfg, err = LoadConfig(env(map[string]string{
		"DOCKVMAP_AGENT_LISTEN":     "10.0.0.5:9100",
		"DOCKVMAP_AGENT_PROXY_ONLY": "true",
		"DOCKER_HOST":               "tcp://socket-proxy:2375",
		"DOCKVMAP_LOG_LEVEL":        "debug",
	}))

	if err != nil || cfg.Listen != "10.0.0.5:9100" || !cfg.ProxyOnly || cfg.DockerHost != "tcp://socket-proxy:2375" || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("custom = %+v, err = %v", cfg, err)
	}

	bad := map[string]map[string]string{
		"must be an address like :9002": {"DOCKVMAP_AGENT_LISTEN": "9002"},
		"must be true or false":         {"DOCKVMAP_AGENT_PROXY_ONLY": "maybe"},
		"must be debug, info, warn":     {"DOCKVMAP_LOG_LEVEL": "loud"},
	}

	for want, values := range bad {
		if _, err := LoadConfig(env(values)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: err = %v, want it to mention %q", values, err, want)
		}
	}
}
