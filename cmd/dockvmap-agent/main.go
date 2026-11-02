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
var defaultDataPath = "./data"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	dataPath := flag.String("data-path", defaultDataPath, "path to the data directory (agent identity and pairing)")
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

	if err := os.MkdirAll(*dataPath, 0o700); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(2)
	}

	cfg.DataPath = *dataPath

	a, err := agent.New(cfg, version)

	if err != nil {
		slog.Error(err.Error())
		os.Exit(2)
	}

	slog.Info("starting dockvmap agent", "version", version, "listen", cfg.Listen, "docker", cfg.DockerHost, "proxyOnly", cfg.ProxyOnly, "fingerprint", a.Fingerprint())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err = a.Run(ctx)
	stop()

	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
