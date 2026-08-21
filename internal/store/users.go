package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

var ErrUsernameConflict = errors.New("username already exists")

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var count int

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}

	return count, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, email, password_hash, preferences, created_at, updated_at FROM users
	`)

	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	defer rows.Close()

	users := make([]model.User, 0)

	for rows.Next() {
		var (
			user        model.User
			preferences string
		)

		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &preferences, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}

		if err := json.Unmarshal([]byte(preferences), &user.Preferences); err != nil {
			return nil, fmt.Errorf("decoding preferences for user %d: %w", user.ID, err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading user rows: %w", err)
	}

	return users, nil
}

func (s *Store) CreateUser(ctx context.Context, user *model.User) error {
	now := time.Now().UTC()

	preferences, err := json.Marshal(user.Preferences)

	if err != nil {
		return fmt.Errorf("encoding preferences: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, email, password_hash, preferences, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, user.Username, user.Email, user.PasswordHash, string(preferences), now, now)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrUsernameConflict
		}

		return fmt.Errorf("creating user: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return fmt.Errorf("getting created user id: %w", err)
	}

	user.ID = id
	user.CreatedAt = now
	user.UpdatedAt = now

	return nil
}

func (s *Store) GetUserByID(ctx context.Context, userID int64) (*model.User, error) {
	var (
		user        model.User
		preferences string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, preferences, created_at, updated_at FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &preferences, &user.CreatedAt, &user.UpdatedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting user %d: %w", userID, err)
	}

	if err := json.Unmarshal([]byte(preferences), &user.Preferences); err != nil {
		return nil, fmt.Errorf("decoding preferences for user %d: %w", userID, err)
	}

	return &user, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var (
		user        model.User
		preferences string
	)

	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, preferences, created_at, updated_at FROM users WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash, &preferences, &user.CreatedAt, &user.UpdatedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting user %q: %w", username, err)
	}

	if err := json.Unmarshal([]byte(preferences), &user.Preferences); err != nil {
		return nil, fmt.Errorf("decoding preferences for user %q: %w", username, err)
	}

	return &user, nil
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID int64, passwordHash string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, passwordHash, userID)

	if err != nil {
		return false, fmt.Errorf("updating password for user %d: %w", userID, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated user %d: %w", userID, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateUserEmail(ctx context.Context, userID int64, email string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET email = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, email, userID)

	if err != nil {
		return false, fmt.Errorf("updating email for user %d: %w", userID, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated user %d: %w", userID, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateUserPreferences(ctx context.Context, userID int64, preferences model.UserPreferences) (bool, error) {
	encoded, err := json.Marshal(preferences)

	if err != nil {
		return false, fmt.Errorf("encoding preferences: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE users SET preferences = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, string(encoded), userID)

	if err != nil {
		return false, fmt.Errorf("updating preferences for user %d: %w", userID, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated user %d: %w", userID, err)
	}

	return updated != 0, nil
}
