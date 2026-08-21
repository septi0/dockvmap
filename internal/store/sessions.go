package store

import (
	"context"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) CreateSession(ctx context.Context, token string, userID int64, ip, userAgent string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token, user_id, ip, user_agent, expires_at) VALUES (?, ?, ?, ?, ?)
	`, token, userID, ip, userAgent, expiresAt)

	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	return nil
}

func (s *Store) GetSessionUser(ctx context.Context, token string) (*model.CurrentUser, error) {
	var user model.CurrentUser

	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?
	`, token, time.Now().UTC()).Scan(&user.ID, &user.Username)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}

	return &user, nil
}

func (s *Store) ListSessionsByUser(ctx context.Context, userID int64, currentToken string) ([]model.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ip, user_agent, created_at, expires_at, token = ?
		FROM sessions
		WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC
	`, currentToken, userID, time.Now().UTC())

	if err != nil {
		return nil, fmt.Errorf("listing sessions for user %d: %w", userID, err)
	}

	defer rows.Close()

	sessions := make([]model.Session, 0)

	for rows.Next() {
		var session model.Session

		if err := rows.Scan(&session.ID, &session.IP, &session.UserAgent, &session.CreatedAt, &session.ExpiresAt, &session.Current); err != nil {
			return nil, fmt.Errorf("scanning session row: %w", err)
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading session rows: %w", err)
	}

	return sessions, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)

	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	return nil
}

func (s *Store) DeleteSessionByID(ctx context.Context, userID, sessionID int64, exceptToken string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions WHERE id = ? AND user_id = ? AND token != ?
	`, sessionID, userID, exceptToken)

	if err != nil {
		return false, fmt.Errorf("deleting session %d: %w", sessionID, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted session %d: %w", sessionID, err)
	}

	return deleted != 0, nil
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", time.Now().UTC())

	if err != nil {
		return 0, fmt.Errorf("deleting expired sessions: %w", err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return 0, fmt.Errorf("checking deleted expired sessions: %w", err)
	}

	return deleted, nil
}

func (s *Store) DeleteOtherSessions(ctx context.Context, userID int64, exceptToken string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token != ?", userID, exceptToken)

	if err != nil {
		return fmt.Errorf("deleting other sessions for user %d: %w", userID, err)
	}

	return nil
}
