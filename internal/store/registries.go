package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

var (
	ErrRegistryConflict                  = errors.New("registry already exists")
	ErrRegistryInUse                     = errors.New("registry is used by one or more images")
	ErrCredentialEncryptionNotConfigured = errors.New("registry credential encryption is not configured")
)

func (s *Store) CreateRegistry(ctx context.Context, registry model.Registry) (int64, error) {
	if registry.Credential != "" && s.credentialCipher == nil {
		return 0, ErrCredentialEncryptionNotConfigured
	}

	now := time.Now().UTC()

	optionsJSON, err := json.Marshal(registry.Options)
	if err != nil {
		return 0, fmt.Errorf("marshalling registry options: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return 0, fmt.Errorf("starting registry creation transaction: %w", err)
	}

	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO registries (registry, username, options, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, registry.Registry, registry.Username, string(optionsJSON), now, now)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return 0, ErrRegistryConflict
		}

		return 0, fmt.Errorf("saving registry: %w", err)
	}

	registryID, err := result.LastInsertId()

	if err != nil {
		return 0, fmt.Errorf("getting created registry id: %w", err)
	}

	if registry.Credential != "" {
		nonce, ciphertext, err := s.encryptRegistryCredential(registryID, registry.Credential)

		if err != nil {
			return 0, err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE registries
			SET credential_nonce = ?, credential_ciphertext = ?
			WHERE id = ?
		`, nonce, ciphertext, registryID); err != nil {
			return 0, fmt.Errorf("saving registry credentials: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing registry creation: %w", err)
	}

	return registryID, nil
}

func (s *Store) GetRegistryCredentials(ctx context.Context, registry string) (*model.RegistryCredentials, error) {
	var (
		id         int64
		username   sql.NullString
		nonce      []byte
		ciphertext []byte
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, credential_nonce, credential_ciphertext
		FROM registries
		WHERE registry = ?
	`, registry).Scan(
		&id,
		&username,
		&nonce,
		&ciphertext,
	)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting registry credentials for %q: %w", registry, err)
	}

	credentials := &model.RegistryCredentials{
		Username: username.String,
	}

	if len(ciphertext) == 0 {
		return credentials, nil
	}

	if s.credentialCipher == nil {
		return nil, ErrCredentialEncryptionNotConfigured
	}

	credential, err := s.credentialCipher.gcm.Open(
		nil,
		nonce,
		ciphertext,
		[]byte(strconv.FormatInt(id, 10)),
	)

	if err != nil {
		return nil, fmt.Errorf("decrypting registry credentials for %q: %w", registry, err)
	}

	credentials.Credential = string(credential)

	return credentials, nil
}

func (s *Store) GetRegistryOptions(ctx context.Context, registry string) (*model.RegistryOptions, error) {
	var optionsJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT options FROM registries WHERE registry = ?
	`, registry).Scan(&optionsJSON)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting registry options: %w", err)
	}

	options := parseRegistryOptions(optionsJSON.String)
	return &options, nil
}

func (s *Store) ListRegistryInfo(ctx context.Context) ([]model.RegistryInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, registry, username, options
		FROM registries
		ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("listing registries: %w", err)
	}

	defer rows.Close()

	registries := make([]model.RegistryInfo, 0)

	for rows.Next() {
		var registry model.RegistryInfo
		var username sql.NullString
		var optionsJSON sql.NullString

		if err := rows.Scan(&registry.ID, &registry.Registry, &username, &optionsJSON); err != nil {
			return nil, fmt.Errorf("scanning registries: %w", err)
		}

		if username.Valid {
			registry.Username = username.String
		}

		registry.Options = parseRegistryOptions(optionsJSON.String)
		registry.AuthenticationConfigured = registry.Username != ""
		registries = append(registries, registry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing registries: %w", err)
	}

	return registries, nil
}

func (s *Store) GetRegistryInfoByID(ctx context.Context, registryID int64) (*model.RegistryInfo, error) {
	var info model.RegistryInfo
	var username sql.NullString
	var optionsJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, registry, username, options FROM registries WHERE id = ?
	`, registryID).Scan(&info.ID, &info.Registry, &username, &optionsJSON)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting registry info by id: %w", err)
	}

	if username.Valid {
		info.Username = username.String
	}

	info.Options = parseRegistryOptions(optionsJSON.String)
	info.AuthenticationConfigured = info.Username != ""

	return &info, nil
}

func (s *Store) GetRegistryInfoByHost(ctx context.Context, registry string) (*model.RegistryInfo, error) {
	var info model.RegistryInfo
	var username sql.NullString
	var optionsJSON sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, registry, username, options FROM registries WHERE registry = ?
	`, registry).Scan(&info.ID, &info.Registry, &username, &optionsJSON)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting registry info by host: %w", err)
	}

	if username.Valid {
		info.Username = username.String
	}

	info.Options = parseRegistryOptions(optionsJSON.String)
	info.AuthenticationConfigured = info.Username != ""

	return &info, nil
}

func (s *Store) UpdateRegistryByID(ctx context.Context, registry model.RegistryUpdate) (bool, error) {
	now := time.Now().UTC()

	query := `
		UPDATE registries
		SET registry = ?, updated_at = ?
	`

	args := []any{
		registry.Registry,
		now,
	}

	if registry.Options != nil {
		optionsJSON, err := json.Marshal(*registry.Options)
		if err != nil {
			return false, fmt.Errorf("marshalling registry options: %w", err)
		}

		query += `, options = ?`
		args = append(args, string(optionsJSON))
	}

	switch {
	case registry.Username == nil && registry.Credential == nil:
	case *registry.Username == "" && *registry.Credential == "":
		query += `, username = NULL, credential_nonce = NULL, credential_ciphertext = NULL`

	default:
		if s.credentialCipher == nil {
			return false, ErrCredentialEncryptionNotConfigured
		}

		nonce, ciphertext, err := s.encryptRegistryCredential(registry.ID, *registry.Credential)

		if err != nil {
			return false, err
		}

		query += `, username = ?, credential_nonce = ?, credential_ciphertext = ?`

		args = append(args, *registry.Username, nonce, ciphertext)
	}

	query += ` WHERE id = ?`
	args = append(args, registry.ID)

	result, err := s.db.ExecContext(ctx, query, args...)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, ErrRegistryConflict
		}

		return false, fmt.Errorf("updating registry by id: %w", err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated registry by id: %w", err)
	}

	return updated != 0, nil
}

func (s *Store) DeleteRegistryByID(ctx context.Context, registryID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM registries WHERE id = ?", registryID)

	if err != nil {
		if isForeignKeyConstraintErr(err) {
			return false, ErrRegistryInUse
		}

		return false, fmt.Errorf("deleting registry by id: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking deleted registry by id: %w", err)
	}

	return deleted != 0, nil
}

func (s *Store) encryptRegistryCredential(registryID int64, credential string) ([]byte, []byte, error) {
	if credential == "" {
		return nil, nil, nil
	}

	if s.credentialCipher == nil {
		return nil, nil, ErrCredentialEncryptionNotConfigured
	}

	nonce := make([]byte, s.credentialCipher.gcm.NonceSize())

	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generating credential nonce: %w", err)
	}

	aad := []byte(strconv.FormatInt(registryID, 10))

	ciphertext := s.credentialCipher.gcm.Seal(
		nil,
		nonce,
		[]byte(credential),
		aad,
	)

	return nonce, ciphertext, nil
}

func parseRegistryOptions(raw string) model.RegistryOptions {
	var options model.RegistryOptions
	if raw == "" {
		return options
	}

	if err := json.Unmarshal([]byte(raw), &options); err != nil {
		return options
	}

	return options
}
