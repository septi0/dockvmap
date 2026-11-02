package agent

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/septi0/dockvmap/internal/agentapi"
)

const defaultDockerHost = "unix:///var/run/docker.sock"

type Config struct {
	Listen     string
	DataPath   string
	ProxyOnly  bool
	DockerHost string
	LogLevel   slog.Level
}

func LoadConfig(getenv func(string) string) (Config, error) {
	var cfg Config

	cfg.Listen = strings.TrimSpace(getenv("DOCKVMAP_AGENT_LISTEN"))

	if cfg.Listen == "" {
		cfg.Listen = ":" + agentapi.DefaultPort
	}

	if _, port, err := net.SplitHostPort(cfg.Listen); err != nil || port == "" {
		return cfg, fmt.Errorf("DOCKVMAP_AGENT_LISTEN must be an address like :%s or 0.0.0.0:%s, got %q", agentapi.DefaultPort, agentapi.DefaultPort, cfg.Listen)
	}

	if raw := strings.TrimSpace(getenv("DOCKVMAP_AGENT_PROXY_ONLY")); raw != "" {
		var err error

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
