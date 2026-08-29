package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

var version = "0.0.0-dev"
var defaultDataPath = "./data" // overridden via -ldflags at Docker build time to match the image's data dir

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to configuration file (optional; falls back to DOCKVMAP_* env vars and defaults if unset, but a specified path that doesn't exist is a startup error)")
	dataPath := flag.String("data-path", defaultDataPath, "path to the data directory (SQLite database, blob cache, credential encryption key)")
	resetPassword := flag.String("reset-password", "", "generate a new random password for the given username, print it, and exit")
	refreshTags := flag.Bool("refresh-tags", false, "refresh tags for all configured images from their upstream registries, then exit")
	backupPath := flag.String("backup", "", "write a consistent copy of the database to the given path, then exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(*configPath)

	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	tagFilter, err := tagfilter.Load(cfg.TagFiltersPath)

	if err != nil {
		return fmt.Errorf("failed to load tag filters: %w", err)
	}

	logFile, err := setupLogging(cfg.LogsPath)

	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	if logFile != nil {
		defer logFile.Close()
	}

	slog.Info("starting dockvmap", "version", version)

	for _, warning := range cfg.DerivedWarnings {
		slog.Warn(warning)
	}

	if err := os.MkdirAll(*dataPath, 0o700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	credentialEncryptionKey, err := resolveCredentialEncryptionKey(cfg, *dataPath)

	if err != nil {
		return fmt.Errorf("failed to resolve credential encryption key: %w", err)
	}

	dbPath := filepath.Join(*dataPath, "dockvmap.db")
	ctx := context.Background()

	// -backup opens the database read-only, so it runs before store.New / migrations
	if *backupPath != "" {
		return cmdBackup(ctx, dbPath, credentialEncryptionKey, *backupPath, cfg, *dataPath)
	}

	db, err := store.New(dbPath, credentialEncryptionKey)

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	defer db.Close()

	switch {
	case *resetPassword != "":
		return cmdResetPassword(ctx, db, cfg, *resetPassword)
	case *refreshTags:
		return cmdRefreshTags(ctx, db, tagFilter)
	default:
		return serve(ctx, cfg, db, *dataPath, tagFilter)
	}
}

func setupLogging(logsPath string) (*os.File, error) {
	if logsPath == "" {
		return nil, nil
	}

	if err := os.MkdirAll(logsPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating logs directory: %w", err)
	}

	logFile, err := os.OpenFile(filepath.Join(logsPath, "dockvmap.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile), nil))
	slog.SetDefault(logger)

	return logFile, nil
}
