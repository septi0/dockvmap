package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

var ErrImageNameConflict = errors.New("image name already exists")

func imageListWhere(filters model.ImageListFilters) (string, []any) {
	b := &whereBuilder{}

	if search := strings.TrimSpace(filters.Search); search != "" {
		term := likeTerm(search)
		b.add(
			`(i.name LIKE ? ESCAPE '\' OR i.repository LIKE ? ESCAPE '\' OR r.registry LIKE ? ESCAPE '\')`,
			term, term, term,
		)
	}

	if filters.UpdateAvailable != nil {
		b.add(`i.update_available = ?`, *filters.UpdateAvailable)
	}

	return b.clause(), b.args
}

func (s *Store) ListImages(ctx context.Context, filters model.ImageListFilters) ([]model.Image, error) {
	where, args := imageListWhere(filters)

	query := fmt.Sprintf(`
		SELECT i.id, i.name, i.registry_id, r.registry, i.repository, i.tag, i.last_checked, i.last_check_error, i.update_available, i.created_at, i.updated_at
		FROM images i
		LEFT JOIN registries r ON r.id = i.registry_id
		%s
		ORDER BY i.id DESC
		LIMIT ? OFFSET ?
	`, where)

	rows, err := s.db.QueryContext(ctx, query, append(args, filters.Limit, filters.Offset)...)

	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	defer rows.Close()

	images := make([]model.Image, 0)

	for rows.Next() {
		var img model.Image

		if err := rows.Scan(
			&img.ID, &img.Name, &img.RegistryID, &img.Registry, &img.Repository, &img.Tag,
			&img.LastChecked, &img.LastCheckError, &img.UpdateAvailable,
			&img.CreatedAt, &img.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning image row: %w", err)
		}

		images = append(images, img)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading image rows: %w", err)
	}

	return images, nil
}

func (s *Store) CountImages(ctx context.Context, filters model.ImageListFilters) (int64, error) {
	where, args := imageListWhere(filters)

	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM images i
		LEFT JOIN registries r ON r.id = i.registry_id
		%s
	`, where)

	var count int64

	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting images: %w", err)
	}

	return count, nil
}

func (s *Store) GetImage(ctx context.Context, name string) (*model.Image, error) {
	var img model.Image

	err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.name, i.registry_id, r.registry, i.repository, i.tag, i.last_checked, i.last_check_error, i.update_available, i.created_at, i.updated_at
		FROM images i
		LEFT JOIN registries r ON r.id = i.registry_id
		WHERE i.name = ?
	`, name).Scan(
		&img.ID, &img.Name, &img.RegistryID, &img.Registry, &img.Repository, &img.Tag,
		&img.LastChecked, &img.LastCheckError, &img.UpdateAvailable,
		&img.CreatedAt, &img.UpdatedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("getting image %q: %w", name, err)
	}

	return &img, nil
}

func (s *Store) GetImageByID(ctx context.Context, imageId int64) (*model.Image, error) {
	var img model.Image

	err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.name, i.registry_id, r.registry, i.repository, i.tag, i.last_checked, i.last_check_error, i.update_available, i.created_at, i.updated_at
		FROM images i
		LEFT JOIN registries r ON r.id = i.registry_id
		WHERE i.id = ?
	`, imageId).Scan(
		&img.ID, &img.Name, &img.RegistryID, &img.Registry, &img.Repository, &img.Tag,
		&img.LastChecked, &img.LastCheckError, &img.UpdateAvailable,
		&img.CreatedAt, &img.UpdatedAt,
	)

	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("getting image %d: %w", imageId, err)
	}

	return &img, nil
}

func (s *Store) CreateImage(ctx context.Context, img *model.Image) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO images (name, registry_id, repository, tag)
		VALUES (?, ?, ?, ?)
	`, img.Name, img.RegistryID, img.Repository, img.Tag)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrImageNameConflict
		}

		return fmt.Errorf("creating image: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return fmt.Errorf("getting created image id: %w", err)
	}

	img.ID = id

	return nil
}

func (s *Store) UpdateImageName(ctx context.Context, imageId int64, name string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE images SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, name, imageId)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, ErrImageNameConflict
		}

		return false, fmt.Errorf("renaming image %d: %w", imageId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking renamed image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) DeleteImage(ctx context.Context, imageId int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, "DELETE FROM images WHERE id = ?", imageId)

	if err != nil {
		return false, fmt.Errorf("deleting image %d: %w", imageId, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted image %d: %w", imageId, err)
	}

	return deleted != 0, nil
}

func (s *Store) UpdateImageCheck(ctx context.Context, tx DBTX, imageId int64, checkErr *string, checkedAt time.Time) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET last_checked = ?, last_check_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, checkedAt, checkErr, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image check %d: %w", imageId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateImageTag(ctx context.Context, tx DBTX, imageId int64, tag string) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET tag = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, tag, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image %d tag: %w", imageId, err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateImageUpdateAvailable(ctx context.Context, tx DBTX, imageId int64, available bool) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET update_available = ?
		WHERE id = ?
	`, available, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image %d update_available: %w", imageId, err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}
