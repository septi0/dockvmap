package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/septi0/dockvmap/internal/config"
	"github.com/septi0/dockvmap/internal/oci"
	"github.com/septi0/dockvmap/internal/service"
	"github.com/septi0/dockvmap/internal/store"
	"github.com/septi0/dockvmap/internal/tagfilter"
)

func cmdResetPassword(ctx context.Context, db *store.Store, cfg *config.Config, username string) error {
	if cfg.Follower.Enabled {
		return fmt.Errorf("this DockVMap is a follower of %s and has no web UI or accounts; reset the password on the primary", cfg.Follower.Primary)
	}

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
		Digests:   ociClient,
		Events:    events,
		Audit:     audit,
		Failures:  failureLog,
		TagFilter: tagFilter,
		BgCtx:     ctx,
	})

	refreshed, err := images.RefreshAll(ctx)

	fmt.Printf("Refreshed tags for %d image(s).\n", refreshed)

	return err
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
	fmt.Printf("  - the agent client identity %s\n", filepath.Join(dataPath, agentClientIdentityFile))
	fmt.Println("    (without it, every host agent has to be reset and adopted again)")

	if cfg.Replication.Enabled {
		fmt.Printf("  - the replication identity %s\n", filepath.Join(dataPath, replicationIdentityFile))
		fmt.Println("    (without it, every follower needs a new replication token)")
	}

	fmt.Println("  - your config.yaml, which is not part of this backup")

	return nil
}

func cmdResync(ctx context.Context, db *store.Store, cfg *config.Config, yes bool) error {
	if !cfg.Follower.Enabled {
		return errors.New("-resync only works on a follower; this DockVMap has follower.enabled off")
	}

	local, err := db.ReplicationSnapshot(ctx)

	if err != nil {
		return err
	}

	fmt.Printf("Resyncing replaces this follower's %d virtual images, %d groups, %d registries and %d proxy tokens with the primary's at %s.\n",
		len(local.Images), len(local.Groups), len(local.Registries), len(local.ProxyTokens), cfg.Follower.Primary)

	if cfg.InternalRegistry.Enabled {
		fmt.Println("It also copies what this follower's registry lacks and removes the images the primary's registry doesn't have.")
	}

	if !yes {
		fmt.Print("Type yes to continue: ")
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')

		if strings.TrimSpace(answer) != "yes" {
			return errors.New("resync cancelled")
		}
	}

	if err := db.RequestResync(ctx); err != nil {
		return err
	}

	fmt.Println("Resync requested. A running follower resyncs within half a minute; one that is not running, when it next starts.")

	return nil
}
