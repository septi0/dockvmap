package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
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
	credentialCipher, err := newCredentialCipher(credentialEncryptionKey)

	if err != nil {
		return nil, err
	}

	separator := "?"

	if strings.Contains(path, "?") {
		separator = "&"
	}

	db, err := sql.Open("sqlite", path+separator+"_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")

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

	s := &Store{db: db, credentialCipher: credentialCipher}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()

		return nil, fmt.Errorf("migrating db: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
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
