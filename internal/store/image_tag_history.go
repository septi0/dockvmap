package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) InsertImageTagHistory(ctx context.Context, tx DBTX, imageId int64, tag string, previousTag *string, source model.TagHistorySource) error {
	db := s.executor(tx)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO image_tag_history (image_id, tag, previous_tag, source)
		VALUES (?, ?, ?, ?)
	`, imageId, tag, previousTag, string(source)); err != nil {
		return fmt.Errorf("recording tag history for image %d: %w", imageId, err)
	}

	return nil
}

func (s *Store) GetImageTagHistory(ctx context.Context, imageId int64) ([]model.ImageTagHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, image_id, tag, previous_tag, source, applied_at
		FROM image_tag_history
		WHERE image_id = ?
		ORDER BY applied_at DESC, id DESC
	`, imageId)

	if err != nil {
		return nil, fmt.Errorf("listing tag history for image %d: %w", imageId, err)
	}

	defer rows.Close()

	history := make([]model.ImageTagHistory, 0)

	for rows.Next() {
		var h model.ImageTagHistory
		var previousTag sql.NullString
		var source string

		if err := rows.Scan(&h.ID, &h.ImageID, &h.Tag, &previousTag, &source, &h.AppliedAt); err != nil {
			return nil, fmt.Errorf("scanning tag history row: %w", err)
		}

		if previousTag.Valid {
			h.PreviousTag = &previousTag.String
		}

		h.Source = model.TagHistorySource(source)

		history = append(history, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading tag history rows: %w", err)
	}

	return history, nil
}
