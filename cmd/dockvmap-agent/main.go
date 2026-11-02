package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/septi0/dockvmap/internal/agent"
)

var version = "0.0.0-dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := agent.LoadConfig(os.Getenv)

	if err != nil {
		fmt.Fprintln(os.Stderr, "dockvmap-agent:", err)
		os.Exit(2)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	a, err := agent.New(cfg, version)

	if err != nil {
		slog.Error(err.Error())
		os.Exit(2)
	}

	slog.Info("starting dockvmap agent", "version", version, "dockvmap", cfg.URL.Redacted(), "docker", cfg.DockerHost, "proxyOnly", cfg.ProxyOnly)

	if cfg.URL.Scheme == "http" {
		slog.Warn("DOCKVMAP_URL uses plain http; the agent token travels unencrypted, use https outside a trusted network")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.Run(ctx)
}
