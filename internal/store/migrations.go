package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

// refuses to proceed if the database was migrated by a binary newer than this one
func (s *Store) verifySchemaVersion(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			checksum TEXT
		)
	`); err != nil {
		return fmt.Errorf("creating migration table: %w", err)
	}

	migrations, err := loadMigrations()

	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		return nil
	}

	highestBundled := migrations[len(migrations)-1].version

	var highestApplied int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&highestApplied); err != nil {
		return fmt.Errorf("checking applied schema version: %w", err)
	}

	if highestApplied > highestBundled {
		return fmt.Errorf("database schema is at version %d but this binary only knows up to %d; run a build at or newer than the one that created this database", highestApplied, highestBundled)
	}

	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	if err := s.verifySchemaVersion(ctx); err != nil {
		return err
	}

	migrations, err := loadMigrations()

	if err != nil {
		return err
	}

	for _, migration := range migrations {
		tx, err := s.db.BeginTx(ctx, nil)

		if err != nil {
			return fmt.Errorf("starting migration %d: %w", migration.version, err)
		}

		var appliedChecksum sql.NullString
		err = tx.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", migration.version).Scan(&appliedChecksum)

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			tx.Rollback()

			return fmt.Errorf("checking migration %d: %w", migration.version, err)
		}

		if err == nil {
			if appliedChecksum.Valid && appliedChecksum.String != migration.checksum {
				tx.Rollback()

				return fmt.Errorf("migration %d (%s) has changed after being applied", migration.version, migration.name)
			}

			if !appliedChecksum.Valid {
				if _, err := tx.ExecContext(ctx, "UPDATE schema_migrations SET checksum = ? WHERE version = ?", migration.checksum, migration.version); err != nil {
					tx.Rollback()

					return fmt.Errorf("backfilling migration %d checksum: %w", migration.version, err)
				}
			}

			if err := tx.Rollback(); err != nil {
				return fmt.Errorf("closing migration %d transaction: %w", migration.version, err)
			}

			continue
		}

		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			tx.Rollback()

			return fmt.Errorf("applying migration %d (%s): %w", migration.version, migration.name, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, checksum) VALUES (?, ?)", migration.version, migration.checksum); err != nil {
			tx.Rollback()

			return fmt.Errorf("recording migration %d: %w", migration.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", migration.version, err)
		}

		slog.Info("applied migration", "version", migration.version, "name", migration.name)
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	paths, err := fs.Glob(migrationFiles, "migrations/*.sql")

	if err != nil {
		return nil, fmt.Errorf("listing migrations: %w", err)
	}

	migrations := make([]migration, 0, len(paths))
	versions := make(map[int]struct{}, len(paths))

	for _, path := range paths {
		name := filepath.Base(path)
		versionText, _, found := strings.Cut(name, "_")

		if !found {
			return nil, fmt.Errorf("migration %q must begin with a numeric version followed by an underscore", name)
		}

		version, err := strconv.Atoi(versionText)

		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q has an invalid version", name)
		}

		if _, exists := versions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}

		versions[version] = struct{}{}

		sql, err := migrationFiles.ReadFile(path)

		if err != nil {
			return nil, fmt.Errorf("reading migration %q: %w", name, err)
		}

		checksum := sha256.Sum256(sql)
		migrations = append(migrations, migration{version: version, name: name, sql: string(sql), checksum: hex.EncodeToString(checksum[:])})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}
