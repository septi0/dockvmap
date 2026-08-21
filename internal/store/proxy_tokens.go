package store

import (
	"context"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) CreateProxyToken(ctx context.Context, label, tokenHash string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO proxy_tokens (label, token_hash) VALUES (?, ?)
	`, label, tokenHash)

	if err != nil {
		return 0, fmt.Errorf("creating proxy token: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return 0, fmt.Errorf("getting created proxy token id: %w", err)
	}

	return id, nil
}

func (s *Store) ListProxyTokens(ctx context.Context) ([]model.ProxyToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, label, created_at FROM proxy_tokens ORDER BY created_at DESC
	`)

	if err != nil {
		return nil, fmt.Errorf("listing proxy tokens: %w", err)
	}

	defer rows.Close()

	tokens := make([]model.ProxyToken, 0)

	for rows.Next() {
		var token model.ProxyToken

		if err := rows.Scan(&token.ID, &token.Label, &token.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning proxy token row: %w", err)
		}

		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading proxy token rows: %w", err)
	}

	return tokens, nil
}

func (s *Store) GetProxyTokenByID(ctx context.Context, id int64) (*model.ProxyToken, error) {
	var token model.ProxyToken

	err := s.db.QueryRowContext(ctx, `
		SELECT id, label, created_at FROM proxy_tokens WHERE id = ?
	`, id).Scan(&token.ID, &token.Label, &token.CreatedAt)

	if isNoRows(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting proxy token by id: %w", err)
	}

	return &token, nil
}

func (s *Store) ProxyTokenHashExists(ctx context.Context, tokenHash string) (bool, error) {
	var exists bool

	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM proxy_tokens WHERE token_hash = ?)
	`, tokenHash).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("checking proxy token: %w", err)
	}

	return exists, nil
}

func (s *Store) DeleteProxyToken(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM proxy_tokens WHERE id = ?", id)

	if err != nil {
		return false, fmt.Errorf("deleting proxy token %d: %w", id, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted proxy token %d: %w", id, err)
	}

	return deleted != 0, nil
}
