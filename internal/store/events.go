package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/septi0/dockvmap/internal/model"
)

func (s *Store) AddTagsEvent(ctx context.Context, imageID int64, eventType string, notify bool, data model.TagsEventData) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding tags event data: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tags_events (image_id, type, data, notify)
		VALUES (?, ?, ?, ?)
	`, imageID, eventType, string(payload), notify)

	if err != nil {
		return fmt.Errorf("adding tags event: %w", err)
	}

	return nil
}

func (s *Store) ListTagsEvents(ctx context.Context, offset, limit int) ([]model.ImageEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ie.id, ie.image_id, i.name, ie.type, ie.data, ie.created_at, ie.notify, ie.notif_sent_at
		FROM tags_events ie
		LEFT JOIN images i ON ie.image_id = i.id
		ORDER BY ie.created_at DESC, ie.id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("querying tags events: %w", err)
	}
	defer rows.Close()

	events := make([]model.ImageEvent, 0, limit)

	for rows.Next() {
		var event model.ImageEvent
		var data string

		if err := rows.Scan(&event.ID, &event.ImageID,
			&event.ImageName, &event.Type, &data,
			&event.CreatedAt, &event.Notify,
			&event.NotifSentAt); err != nil {
			return nil, fmt.Errorf("scanning tags event: %w", err)
		}

		if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
			return nil, fmt.Errorf("decoding tags event data: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tags events: %w", err)
	}

	return events, nil
}

func (s *Store) ListPendingTagNotificationEvents(ctx context.Context, limit int) ([]model.ImageEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ie.id, ie.image_id, i.name, ie.type, ie.data, ie.created_at, ie.notify, ie.notif_sent_at
		FROM tags_events ie
		LEFT JOIN images i ON ie.image_id = i.id
		WHERE ie.notify = 1 AND ie.notif_sent_at IS NULL
		ORDER BY ie.created_at, ie.id
		LIMIT ?
	`, limit)

	if err != nil {
		return nil, fmt.Errorf("querying pending tag notification events: %w", err)
	}

	defer rows.Close()

	events := make([]model.ImageEvent, 0, limit)

	for rows.Next() {
		var event model.ImageEvent
		var data string

		if err := rows.Scan(&event.ID, &event.ImageID,
			&event.ImageName, &event.Type, &data,
			&event.CreatedAt, &event.Notify,
			&event.NotifSentAt); err != nil {
			return nil, fmt.Errorf("scanning pending tag notification event: %w", err)
		}

		if err := json.Unmarshal([]byte(data), &event.Data); err != nil {
			return nil, fmt.Errorf("decoding tags event data: %w", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating pending tag notification events: %w", err)
	}

	return events, nil
}

func (s *Store) MarkTagsEventNotified(ctx context.Context, eventID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tags_events SET notif_sent_at = CURRENT_TIMESTAMP WHERE id = ?
	`, eventID)

	if err != nil {
		return fmt.Errorf("marking tags event %d notified: %w", eventID, err)
	}

	return nil
}
