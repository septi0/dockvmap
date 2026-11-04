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

const imageSelect = `
	SELECT i.id, i.name, i.registry_id, r.registry, i.repository, i.tag, i.last_checked, i.last_check_error, i.update_available, i.update_available_tag, i.pinned, i.refresh_status, i.group_id, g.name, i.tag_set_hash, i.created_at, i.updated_at, i.last_pulled_at, i.last_pulled_tag, i.current_digest, i.current_digest_tag, i.current_digest_since, COALESCE(r.internal, 0)
	FROM images i
	LEFT JOIN registries r ON r.id = i.registry_id
	LEFT JOIN image_groups g ON g.id = i.group_id
`

var imageSort = sortSpec{
	columns: map[string]sortColumn{
		model.ImageSortName:        {expr: "i.name"},
		model.ImageSortLastChecked: {expr: "i.last_checked", nullable: true},
		model.ImageSortLastPulled:  {expr: "i.last_pulled_at", nullable: true},
		model.ImageSortStatus: {expr: `CASE
			WHEN i.update_available = 1 THEN 0
			WHEN i.last_check_error IS NOT NULL THEN 1
			WHEN i.pinned = 1 THEN 2
			ELSE 3
		END`},
	},
	fallback: "i.id DESC",
	tieBreak: "i.name ASC, i.id DESC",
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanImage(row rowScanner, img *model.Image) error {
	return row.Scan(
		&img.ID, &img.Name, &img.RegistryID, &img.Registry, &img.Repository, &img.Tag,
		&img.LastChecked, &img.LastCheckError, &img.UpdateAvailable, &img.UpdateAvailableTag,
		&img.Pinned, &img.RefreshStatus, &img.GroupID, &img.GroupName, &img.TagSetHash, &img.CreatedAt, &img.UpdatedAt,
		&img.LastPulledAt, &img.LastPulledTag,
		&img.CurrentDigest, &img.CurrentDigestTag, &img.CurrentDigestSince, &img.RegistryInternal,
	)
}

func imageListWhere(filters model.ImageListFilters) (string, []any) {
	b := &whereBuilder{}

	if search := strings.TrimSpace(filters.Search); search != "" {
		term := likeTerm(search)
		b.add(
			`(i.name LIKE ? ESCAPE '\' OR i.repository LIKE ? ESCAPE '\' OR r.registry LIKE ? ESCAPE '\' OR g.name LIKE ? ESCAPE '\')`,
			term, term, term, term,
		)
	}

	switch filters.Status {
	case model.ImageStatusUpdateAvailable:
		b.add(`i.update_available = 1`)
	case model.ImageStatusFailedCheck:
		b.add(`i.last_check_error IS NOT NULL`)
	case model.ImageStatusPinned:
		b.add(`i.pinned = 1`)
	}

	if filters.Ungrouped {
		b.add(`i.group_id IS NULL`)
	}

	if filters.GroupID > 0 {
		b.add(`i.group_id = ?`, filters.GroupID)
	}

	return b.clause(), b.args
}

func (s *Store) ListImages(ctx context.Context, filters model.ImageListFilters) ([]model.Image, error) {
	where, args := imageListWhere(filters)

	query := fmt.Sprintf(`%s
		%s
		%s
		LIMIT ? OFFSET ?
	`, imageSelect, where, imageSort.orderBy(filters.Sort))

	rows, err := s.db.QueryContext(ctx, query, append(args, filters.Limit, filters.Offset)...)

	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
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

func (s *Store) ListImageIDsByRegistryRepository(ctx context.Context, registryID int64, repository string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM images WHERE registry_id = ? AND repository = ? ORDER BY id ASC
	`, registryID, repository)

	if err != nil {
		return nil, fmt.Errorf("listing images of %q: %w", repository, err)
	}

	defer rows.Close()

	ids := make([]int64, 0)

	for rows.Next() {
		var id int64

		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning image row: %w", err)
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading image rows: %w", err)
	}

	return ids, nil
}

func (s *Store) ListImagesAfterID(ctx context.Context, afterID int64, limit int) ([]model.Image, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name
		FROM images
		WHERE id > ?
		ORDER BY id ASC
		LIMIT ?
	`, afterID, limit)

	if err != nil {
		return nil, fmt.Errorf("listing images after %d: %w", afterID, err)
	}

	defer rows.Close()

	images := make([]model.Image, 0)

	for rows.Next() {
		var img model.Image

		if err := rows.Scan(&img.ID, &img.Name); err != nil {
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
		LEFT JOIN image_groups g ON g.id = i.group_id
		%s
	`, where)

	var count int64

	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting images: %w", err)
	}

	return count, nil
}

func (s *Store) ImageStatusCounts(ctx context.Context) (model.ImageStatusCounts, error) {
	var counts model.ImageStatusCounts

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN update_available = 1 AND group_id IS NULL THEN 1 ELSE 0 END), 0)
				+ COUNT(DISTINCT CASE WHEN update_available = 1 THEN group_id END),
			COALESCE(SUM(CASE WHEN last_check_error IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM images
	`).Scan(&counts.Total, &counts.UpdateAvailable, &counts.FailedCheck)

	if err != nil {
		return model.ImageStatusCounts{}, fmt.Errorf("counting image statuses: %w", err)
	}

	return counts, nil
}

func (s *Store) GetImage(ctx context.Context, name string) (*model.Image, error) {
	var img model.Image

	err := scanImage(s.db.QueryRowContext(ctx, imageSelect+`WHERE i.name = ?`, name), &img)

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

	err := scanImage(s.db.QueryRowContext(ctx, imageSelect+`WHERE i.id = ?`, imageId), &img)

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

func (s *Store) UpdateImageCheck(ctx context.Context, tx DBTX, imageId int64, checkErr *string, checkedAt time.Time, tagSetHash string) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET last_checked = ?, last_check_error = ?, tag_set_hash = ?
		WHERE id = ?
	`, checkedAt, checkErr, tagSetHash, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image check %d: %w", imageId, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) TryStartImageRefresh(ctx context.Context, imageId int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE images
		SET refresh_status = ?
		WHERE id = ? AND refresh_status = ?
	`, model.RefreshStatusRunning, imageId, model.RefreshStatusIdle)

	if err != nil {
		return false, fmt.Errorf("starting image refresh %d: %w", imageId, err)
	}

	affected, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking started image refresh %d: %w", imageId, err)
	}

	return affected != 0, nil
}

func (s *Store) SetImageRefreshStatus(ctx context.Context, imageId int64, status string) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE images
		SET refresh_status = ?
		WHERE id = ?
	`, status, imageId); err != nil {
		return fmt.Errorf("setting image refresh status %d: %w", imageId, err)
	}

	return nil
}

func (s *Store) ResetRunningImageRefreshes(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE images
		SET refresh_status = ?
		WHERE refresh_status = ?
	`, model.RefreshStatusIdle, model.RefreshStatusRunning)

	if err != nil {
		return 0, fmt.Errorf("resetting running image refreshes: %w", err)
	}

	return result.RowsAffected()
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

func (s *Store) UpdateImagePinned(ctx context.Context, tx DBTX, imageId int64, pinned bool) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET pinned = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, pinned, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image %d pinned: %w", imageId, err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateImageUpdateAvailable(ctx context.Context, tx DBTX, imageId int64, available bool, targetTag *string) (bool, error) {
	db := s.executor(tx)

	result, err := db.ExecContext(ctx, `
		UPDATE images
		SET update_available = ?, update_available_tag = ?
		WHERE id = ?
	`, available, targetTag, imageId)

	if err != nil {
		return false, fmt.Errorf("updating image %d update_available: %w", imageId, err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking updated image %d: %w", imageId, err)
	}

	return updated != 0, nil
}

func (s *Store) UpdateImageDigest(ctx context.Context, imageId int64, tag string, digest *string, seenAt time.Time) error {
	var since *time.Time

	if digest != nil {
		since = &seenAt
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE images
		SET current_digest = ?, current_digest_tag = ?, current_digest_since = ?
		WHERE id = ? AND tag = ? AND (current_digest IS NOT ? OR current_digest_tag IS NOT ?)
	`, digest, tag, since, imageId, tag, digest, tag)

	if err != nil {
		return fmt.Errorf("updating image %d digest: %w", imageId, err)
	}

	return nil
}
