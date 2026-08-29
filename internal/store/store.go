package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type Store struct {
	db               *sql.DB
	credentialCipher *credentialCipher
}

type credentialCipher struct {
	gcm cipher.AEAD
}

func New(path, credentialEncryptionKey string) (*Store, error) {
	s, err := open(path, credentialEncryptionKey)

	if err != nil {
		return nil, err
	}

	if err := s.migrate(context.Background()); err != nil {
		s.db.Close()

		return nil, fmt.Errorf("migrating db: %w", err)
	}

	restrictDBFilePermissions(path)

	return s, nil
}

// OpenForBackup skips migrations so a backup snapshots the schema as found; a newer schema is still refused
func OpenForBackup(path, credentialEncryptionKey string) (*Store, error) {
	s, err := open(path, credentialEncryptionKey)

	if err != nil {
		return nil, err
	}

	if err := s.verifySchemaVersion(context.Background()); err != nil {
		s.db.Close()

		return nil, err
	}

	restrictDBFilePermissions(path)

	return s, nil
}

func open(path, credentialEncryptionKey string) (*Store, error) {
	credentialCipher, err := newCredentialCipher(credentialEncryptionKey)

	if err != nil {
		return nil, err
	}

	separator := "?"

	if strings.Contains(path, "?") {
		separator = "&"
	}

	db, err := sql.Open("sqlite", path+separator+"_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")

	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(context.Background()); err != nil {
		db.Close()

		return nil, fmt.Errorf("connecting db: %w", err)
	}

	restrictDBFilePermissions(path)

	return &Store{db: db, credentialCipher: credentialCipher}, nil
}

func restrictDBFilePermissions(path string) {
	// the DB and its WAL sidecars hold password hashes, session tokens and encrypted credentials
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			slog.Warn("could not restrict database file permissions", "path", p, "error", err)
		}
	}
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Backup(ctx context.Context, destPath string) error {
	// VACUUM INTO takes no bind parameter; destPath is a trusted CLI arg, quote-escaped
	escaped := strings.ReplaceAll(destPath, "'", "''")

	if _, err := s.db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("backing up database to %q: %w", destPath, err)
	}

	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	var one int

	if err := s.db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database health check: %w", err)
	}

	return nil
}

func (s *Store) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}

	return s.db
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func isUniqueConstraintErr(err error) bool {
	var sqliteErr *sqlite.Error

	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func isForeignKeyConstraintErr(err error) bool {
	var sqliteErr *sqlite.Error

	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

func newCredentialCipher(encodedKey string) (*credentialCipher, error) {
	if encodedKey == "" {
		return nil, nil
	}

	key, err := base64.StdEncoding.DecodeString(encodedKey)

	if err != nil {
		return nil, fmt.Errorf("decoding credential encryption key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("credential encryption key must be a base64-encoded 32-byte key")
	}

	block, err := aes.NewCipher(key)

	if err != nil {
		return nil, fmt.Errorf("creating credential cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return nil, fmt.Errorf("creating credential cipher: %w", err)
	}

	return &credentialCipher{gcm: gcm}, nil
}
