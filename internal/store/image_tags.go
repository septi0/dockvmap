package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

func marshalVersionSegments(segments [][]int64) string {
	if len(segments) == 0 {
		return ""
	}

	encoded, err := json.Marshal(segments)

	if err != nil {
		return ""
	}

	return string(encoded)
}

func scanVersionSegments(raw string) ([][]int64, error) {
	if raw == "" {
		return nil, nil
	}

	var segments [][]int64

	if err := json.Unmarshal([]byte(raw), &segments); err != nil {
		return nil, fmt.Errorf("decoding version segments %q: %w", raw, err)
	}

	return segments, nil
}

func (s *Store) GetImageTags(ctx context.Context, imageId int64) ([]model.ImageTag, error) {
	query := `
		SELECT id, family_id, family_type, family_has_order, tag, tag_order, prerelease, version_segments, first_seen, last_seen, new
		FROM image_tags
		WHERE image_id = ?
		ORDER BY tag_order ASC`

	var rows *sql.Rows
	var err error

	rows, err = s.db.QueryContext(ctx, query, imageId)

	if err != nil {
		return nil, fmt.Errorf("getting image tags for %d: %w", imageId, err)
	}

	defer rows.Close()

	tags := make([]model.ImageTag, 0)

	for rows.Next() {
		var tag model.ImageTag
		var versionSegments string

		if err := rows.Scan(
			&tag.ID,
			&tag.FamilyID,
			&tag.FamilyType,
			&tag.FamilyHasOrder,
			&tag.Tag,
			&tag.TagOrder,
			&tag.Prerelease,
			&versionSegments,
			&tag.FirstSeen,
			&tag.LastSeen,
			&tag.New,
		); err != nil {
			return nil, fmt.Errorf("scanning image tag row: %w", err)
		}

		if tag.VersionSegments, err = scanVersionSegments(versionSegments); err != nil {
			return nil, err
		}

		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading image tag rows: %w", err)
	}

	return tags, nil
}

func (s *Store) GetImageTag(ctx context.Context, imageId int64, name string) (*model.ImageTag, error) {
	var imageTag model.ImageTag
	var versionSegments string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, family_id, family_type, family_has_order, tag, tag_order, prerelease, version_segments, first_seen, last_seen, new
		FROM image_tags
		WHERE image_id = ? AND tag = ?
		LIMIT 1
	`, imageId, name).Scan(
		&imageTag.ID,
		&imageTag.FamilyID,
		&imageTag.FamilyType,
		&imageTag.FamilyHasOrder,
		&imageTag.Tag,
		&imageTag.TagOrder,
		&imageTag.Prerelease,
		&versionSegments,
		&imageTag.FirstSeen,
		&imageTag.LastSeen,
		&imageTag.New,
	)

	if err == nil {
		if imageTag.VersionSegments, err = scanVersionSegments(versionSegments); err != nil {
			return nil, err
		}

		return &imageTag, nil
	}

	if isNoRows(err) {
		return nil, nil
	}

	return nil, fmt.Errorf("getting image tag %q: %w", name, err)
}

func (s *Store) SetImageTags(ctx context.Context, tx DBTX, imageId int64, tags []model.ImageTag) error {
	db := s.executor(tx)

	if len(tags) == 0 {
		return nil
	}

	stmt, err := db.PrepareContext(ctx, `
		INSERT INTO image_tags (image_id, family_id, family_type, family_has_order, tag, tag_order, prerelease, version_segments, first_seen, last_seen, new)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(image_id, tag) DO UPDATE SET
			family_id = excluded.family_id,
    		family_type = excluded.family_type,
			family_has_order = excluded.family_has_order,
			tag_order = excluded.tag_order,
			prerelease = excluded.prerelease,
			version_segments = excluded.version_segments,
			last_seen = excluded.last_seen
	`)

	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}

	defer stmt.Close()

	for _, tag := range tags {
		if _, err := stmt.ExecContext(ctx,
			imageId,
			tag.FamilyID,
			tag.FamilyType,
			tag.FamilyHasOrder,
			tag.Tag,
			tag.TagOrder,
			tag.Prerelease,
			marshalVersionSegments(tag.VersionSegments),
			tag.FirstSeen,
			tag.LastSeen,
			tag.New,
		); err != nil {
			return fmt.Errorf("inserting image tag %q: %w", tag.Tag, err)
		}
	}

	return nil
}

func (s *Store) DeleteImageTagsNotSeen(ctx context.Context, tx DBTX, imageId int64, lastSeen time.Time) (int64, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		DELETE FROM image_tags WHERE image_id = ? AND last_seen < ?
	`, imageId, lastSeen)

	if err != nil {
		return 0, fmt.Errorf("deleting image tags not seen for image %d: %w", imageId, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return 0, fmt.Errorf("checking deleted image tags for image %d: %w", imageId, err)
	}

	return deleted, nil
}

func (s *Store) MarkImageTagsAsSeen(ctx context.Context, imageId int64) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE image_tags
		SET new = 0
		WHERE image_id = ? AND new = 1
	`, imageId)
	if err != nil {
		return 0, fmt.Errorf("marking image tags as seen for image %d: %w", imageId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return 0, fmt.Errorf("checking updated image tags for image %d: %w", imageId, err)
	}

	return updated, nil
}
