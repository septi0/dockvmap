package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

func cmdResetPassword(ctx context.Context, db *store.Store, cfg *config.Config, username string) error {
	loginRateLimiter, err := service.NewLoginRateLimiter(*cfg.LoginRateLimit.Enabled, cfg.LoginRateLimit.MaxAttempts, cfg.LoginRateLimit.WindowDuration(), cfg.LoginRateLimit.BypassIPs)

	if err != nil {
		return fmt.Errorf("failed to configure login rate limiter: %w", err)
	}

	audit := service.NewAudit(db)
	sessions := service.NewSessions(db, cfg.SessionLifetimeDuration(), audit, loginRateLimiter)
	users := service.NewUsers(db, audit, sessions)

	password, err := users.ResetPassword(ctx, username)

	if err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	fmt.Println("Password reset. New password:")
	fmt.Println(password)

	return nil
}

func cmdRefreshTags(ctx context.Context, db *store.Store, tagFilter *tagfilter.Filter) error {
	audit := service.NewAudit(db)
	registries := service.NewRegistries(service.RegistriesDeps{Store: db, Audit: audit})
	ociClient := oci.NewClient(nil, registryCredentialsAdapter{registries: registries}, registryOptionsAdapter{registries: registries})
	failureLog := service.NewFailureLog(db)
	events := service.NewEvents(db)

	images := service.NewImages(service.ImagesDeps{
		Store:     db,
		TagLister: ociClient,
		Events:    events,
		Audit:     audit,
		Failures:  failureLog,
		TagFilter: tagFilter,
		BgCtx:     ctx,
	})

	refreshed, err := images.RefreshAll(ctx)

	fmt.Printf("Refreshed tags for %d image(s).\n", refreshed)

	if err != nil {
		return fmt.Errorf("some images failed to refresh: %w", err)
	}

	return nil
}

func cmdBackup(ctx context.Context, dbPath, credentialEncryptionKey, destPath string, cfg *config.Config, dataPath string) error {
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("backup target %q already exists; choose a path that does not exist yet", destPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking backup target %q: %w", destPath, err)
	}

	db, err := store.OpenForBackup(dbPath, credentialEncryptionKey)

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	defer db.Close()

	if err := db.Backup(ctx, destPath); err != nil {
		return err
	}

	fmt.Printf("Database backup written to %s\n\n", destPath)
	fmt.Println("This backup contains the database only. A full restore also needs:")

	if cfg.CredentialEncryptionKey != "" {
		fmt.Println("  - the credential_encryption_key value from your configuration")
	} else {
		fmt.Printf("  - the key file %s\n", filepath.Join(dataPath, "credential_encryption.key"))
	}

	fmt.Println("    (without it, stored registry credentials cannot be decrypted)")
	fmt.Println("  - your config.yaml, which is not part of this backup")

	return nil
}
