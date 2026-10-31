package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/septi0/dockvmap/internal/model"
)

var ErrImageGroupNameConflict = errors.New("image group name already exists")

func imageGroupListWhere(filters model.ImageGroupListFilters) (string, []any) {
	b := &whereBuilder{}

	if search := strings.TrimSpace(filters.Search); search != "" {
		term := likeTerm(search)
		b.add(
			`(g.name LIKE ? ESCAPE '\' OR EXISTS (
				SELECT 1 FROM images i
				LEFT JOIN registries r ON r.id = i.registry_id
				WHERE i.group_id = g.id
					AND (i.name LIKE ? ESCAPE '\' OR i.repository LIKE ? ESCAPE '\' OR r.registry LIKE ? ESCAPE '\')
			))`,
			term, term, term, term,
		)
	}

	if filters.UpdateAvailable {
		b.add(`EXISTS (SELECT 1 FROM images i WHERE i.group_id = g.id AND i.update_available = 1)`)
	}

	if filters.Pinned {
		b.add(`EXISTS (SELECT 1 FROM images i WHERE i.group_id = g.id AND i.pinned = 1)`)
	}

	return b.clause(), b.args
}

func (s *Store) ListImageGroups(ctx context.Context, filters model.ImageGroupListFilters) ([]model.ImageGroup, error) {
	where, args := imageGroupListWhere(filters)

	query := fmt.Sprintf(`
		SELECT g.id, g.name, g.created_at, g.updated_at
		FROM image_groups g
		%s
		ORDER BY g.name COLLATE NOCASE ASC
		LIMIT ? OFFSET ?
	`, where)

	rows, err := s.db.QueryContext(ctx, query, append(args, filters.Limit, filters.Offset)...)

	if err != nil {
		return nil, fmt.Errorf("listing image groups: %w", err)
	}

	defer rows.Close()

	groups := make([]model.ImageGroup, 0)

	for rows.Next() {
		var group model.ImageGroup

		if err := rows.Scan(&group.ID, &group.Name, &group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning image group row: %w", err)
		}

		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading image group rows: %w", err)
	}

	return groups, nil
}

func (s *Store) CountImageGroups(ctx context.Context, filters model.ImageGroupListFilters) (int64, error) {
	where, args := imageGroupListWhere(filters)

	return s.countWhere(ctx, "image_groups g", where, args)
}

func (s *Store) GetImageGroupByID(ctx context.Context, groupId int64) (*model.ImageGroup, error) {
	var group model.ImageGroup

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, created_at, updated_at
		FROM image_groups
		WHERE id = ?
	`, groupId).Scan(&group.ID, &group.Name, &group.CreatedAt, &group.UpdatedAt)

	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("getting image group %d: %w", groupId, err)
	}

	return &group, nil
}

func (s *Store) ListImageGroupMembers(ctx context.Context, groupId int64) ([]model.Image, error) {
	rows, err := s.db.QueryContext(ctx, imageSelect+`WHERE i.group_id = ? ORDER BY i.id ASC`, groupId)

	if err != nil {
		return nil, fmt.Errorf("listing members of image group %d: %w", groupId, err)
	}

	defer rows.Close()

	images := make([]model.Image, 0)

	for rows.Next() {
		var img model.Image

		if err := scanImage(rows, &img); err != nil {
			return nil, fmt.Errorf("scanning image row: %w", err)
		}

		images = append(images, img)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading image rows: %w", err)
	}

	return images, nil
}

func (s *Store) CreateImageGroup(ctx context.Context, tx DBTX, name string) (int64, error) {
	result, err := s.executor(tx).ExecContext(ctx, `INSERT INTO image_groups (name) VALUES (?)`, name)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return 0, ErrImageGroupNameConflict
		}

		return 0, fmt.Errorf("creating image group: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return 0, fmt.Errorf("getting created image group id: %w", err)
	}

	return id, nil
}

func (s *Store) UpdateImageGroupName(ctx context.Context, tx DBTX, groupId int64, name string) (bool, error) {
	result, err := s.executor(tx).ExecContext(ctx, `
		UPDATE image_groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, name, groupId)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, ErrImageGroupNameConflict
		}

		return false, fmt.Errorf("renaming image group %d: %w", groupId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking renamed image group %d: %w", groupId, err)
	}

	return updated != 0, nil
}

func (s *Store) DeleteImageGroup(ctx context.Context, groupId int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM image_groups WHERE id = ?`, groupId)

	if err != nil {
		return false, fmt.Errorf("deleting image group %d: %w", groupId, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted image group %d: %w", groupId, err)
	}

	return deleted != 0, nil
}

func (s *Store) DeleteImageGroupIfEmpty(ctx context.Context, groupId int64) error {
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM image_groups
		WHERE id = ? AND NOT EXISTS (SELECT 1 FROM images WHERE group_id = ?)
	`, groupId, groupId); err != nil {
		return fmt.Errorf("deleting empty image group %d: %w", groupId, err)
	}

	return nil
}

func (s *Store) UpdateImageGroupMembership(ctx context.Context, tx DBTX, imageId int64, groupId *int64) (bool, error) {
	result, err := s.executor(tx).ExecContext(ctx, `
		UPDATE images SET group_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, groupId, imageId)

	if err != nil {
		return false, fmt.Errorf("updating group of image %d: %w", imageId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}
