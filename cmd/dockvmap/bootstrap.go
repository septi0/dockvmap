package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/septi0/dockvmap/internal/agentapi"
	"github.com/septi0/dockvmap/internal/blobcache"
	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/smtp"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/webhook"
)

const credentialEncryptionKeyFile = "credential_encryption.key"

const agentClientIdentityFile = "agent_client.pem"

func resolveCredentialEncryptionKey(cfg *config.Config, dataPath string) (string, error) {
	if cfg.CredentialEncryptionKey != "" {
		return cfg.CredentialEncryptionKey, nil
	}

	keyPath := filepath.Join(dataPath, credentialEncryptionKeyFile)

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

	if err := writeFileAtomic(keyPath, []byte(encoded), 0o600); err != nil {
		return "", fmt.Errorf("writing credential encryption key: %w", err)
	}

	slog.Info("generated credential encryption key", "path", keyPath)

	return encoded, nil
}

func loadAgentClientIdentity(dataPath string) (tls.Certificate, error) {
	path := filepath.Join(dataPath, agentClientIdentityFile)

	data, err := os.ReadFile(path)

	if errors.Is(err, os.ErrNotExist) {
		if data, err = agentapi.NewIdentity("dockvmap"); err != nil {
			return tls.Certificate{}, fmt.Errorf("creating the agent client identity: %w", err)
		}

		if err := writeFileAtomic(path, data, 0o600); err != nil {
			return tls.Certificate{}, fmt.Errorf("writing the agent client identity: %w", err)
		}

		slog.Info("generated the agent client identity", "path", path)
	} else if err != nil {
		return tls.Certificate{}, fmt.Errorf("reading the agent client identity: %w", err)
	}

	identity, err := agentapi.ParseIdentity(data)

	if err != nil {
		return tls.Certificate{}, fmt.Errorf("reading the agent client identity %s: %w", path, err)
	}

	return identity, nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")

	if err != nil {
		return err
	}

	tmpName := tmp.Name()

	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}

func initBlobCache(cfg *config.Config, dataPath string, db *store.Store) (*blobcache.Cache, error) {
	if !*cfg.BlobCache.Enabled {
		return nil, nil
	}

	cachePath := filepath.Join(dataPath, "cache")

	cache, err := blobcache.New(cachePath, cfg.BlobCache.Lifetime, cfg.BlobCacheMaxSizeBytes(), db)

	if err != nil {
		return nil, fmt.Errorf("failed to initialize blob cache: %w", err)
	}

	slog.Info("blob cache enabled", "path", cachePath, "lifetime", cache.Lifetime())

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
