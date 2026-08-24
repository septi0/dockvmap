package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/smtp"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/webhook"
)

const credentialEncryptionKeyFile = "credential_encryption.key"

func resolveCredentialEncryptionKey(cfg *config.Config) (string, error) {
	if cfg.CredentialEncryptionKey != "" {
		return cfg.CredentialEncryptionKey, nil
	}

	keyPath := filepath.Join(cfg.DataPath, credentialEncryptionKeyFile)

	data, err := os.ReadFile(keyPath)

	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading credential encryption key: %w", err)
	}

	key := make([]byte, 32)

	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generating credential encryption key: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(key)

	if err := os.WriteFile(keyPath, []byte(encoded), 0o600); err != nil {
		return "", fmt.Errorf("writing credential encryption key: %w", err)
	}

	slog.Info("generated credential encryption key", "path", keyPath)

	return encoded, nil
}

func initBlobCache(cfg *config.Config, db *store.Store) (*blobcache.Cache, error) {
	if !*cfg.BlobCache.Enabled {
		return nil, nil
	}

	cache, err := blobcache.New(cfg.BlobCache.Path, cfg.BlobCache.Lifetime, db)

	if err != nil {
		return nil, fmt.Errorf("failed to initialize blob cache: %w", err)
	}

	slog.Info("blob cache enabled", "path", cfg.BlobCache.Path, "lifetime", cache.Lifetime())

	return cache, nil
}

func initMailer(cfg *config.Config) *smtp.Client {
	if !cfg.SMTP.Enabled {
		return nil
	}

	mailer := smtp.NewClient(smtp.Config{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
		TLS:      *cfg.SMTP.TLS,
	})

	slog.Info("smtp enabled", "host", cfg.SMTP.Host, "port", cfg.SMTP.Port)

	return mailer
}

func initNotifications(cfg *config.Config, db *store.Store, mailer *smtp.Client, failureLog *service.FailureLog) (*service.Notifications, error) {
	if len(cfg.Webhooks) > 0 {
		slog.Info("webhook notifications enabled", "count", len(cfg.Webhooks))
	}

	notifications, err := service.NewNotifications(db, db, mailer, cfg.SMTP.Enabled, webhook.NewClient(), cfg.Webhooks, failureLog)

	if err != nil {
		return nil, fmt.Errorf("failed to configure webhook notifications: %w", err)
	}

	return notifications, nil
}
