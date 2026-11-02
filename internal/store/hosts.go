package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/septi0/dockvmap/internal/model"
)

var ErrHostNameConflict = errors.New("host name already exists")

const hostSelect = `
	SELECT h.id, h.name, h.created_at, h.last_seen_at, h.agent_version, h.protocol_version,
		h.hostname, h.docker_version, h.os, h.architecture, h.cpus, h.memory_bytes, h.docker_error, h.fingerprint,
		(SELECT COUNT(*) FROM host_containers c WHERE c.host_id = h.id AND c.state = 'running'),
		(SELECT COUNT(*) FROM host_containers c WHERE c.host_id = h.id AND c.state <> 'running'),
		(SELECT COUNT(DISTINCT c.compose_project) FROM host_containers c WHERE c.host_id = h.id)
	FROM hosts h
`

const hostContainerSelect = `
	SELECT host_id, container_id, name, image, image_host, image_repository, image_tag, image_digest,
		running_digests, ref_digests, state, health, exit_code, compose_project, compose_service,
		created_at, started_at, finished_at, ports
	FROM host_containers
`

func scanHost(row rowScanner, host *model.Host) error {
	return row.Scan(
		&host.ID, &host.Name, &host.CreatedAt, &host.LastSeenAt, &host.AgentVersion, &host.ProtocolVersion,
		&host.Info.Hostname, &host.Info.DockerVersion, &host.Info.OS, &host.Info.Architecture, &host.Info.CPUs,
		&host.Info.MemoryBytes, &host.DockerError, &host.Fingerprint, &host.RunningContainers, &host.StoppedContainers,
		&host.Stacks,
	)
}

func scanHostContainer(row rowScanner, container *model.HostContainer) error {
	var runningDigests, refDigests, ports string

	err := row.Scan(
		&container.HostID, &container.ContainerID, &container.Name, &container.Image, &container.ImageHost,
		&container.ImageRepository, &container.ImageTag, &container.ImageDigest, &runningDigests, &refDigests,
		&container.State, &container.Health, &container.ExitCode, &container.ComposeProject,
		&container.ComposeService, &container.CreatedAt, &container.StartedAt, &container.FinishedAt, &ports,
	)

	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(runningDigests), &container.RunningDigests); err != nil {
		return fmt.Errorf("decoding running digests: %w", err)
	}

	if err := json.Unmarshal([]byte(refDigests), &container.RefDigests); err != nil {
		return fmt.Errorf("decoding reference digests: %w", err)
	}

	if err := json.Unmarshal([]byte(ports), &container.Ports); err != nil {
		return fmt.Errorf("decoding ports: %w", err)
	}

	return nil
}

func marshalList[T any](items []T) string {
	if len(items) == 0 {
		return "[]"
	}

	encoded, err := json.Marshal(items)

	if err != nil {
		return "[]"
	}

	return string(encoded)
}

func (s *Store) CreateHost(ctx context.Context, name, tokenHash string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO hosts (name, token_hash) VALUES (?, ?)`, name, tokenHash)

	if err != nil {
		if isUniqueConstraintErr(err) {
			return 0, ErrHostNameConflict
		}

		return 0, fmt.Errorf("creating host: %w", err)
	}

	id, err := result.LastInsertId()

	if err != nil {
		return 0, fmt.Errorf("getting created host id: %w", err)
	}

	return id, nil
}

func (s *Store) ListHosts(ctx context.Context) ([]model.Host, error) {
	rows, err := s.db.QueryContext(ctx, hostSelect+`ORDER BY h.name`)

	if err != nil {
		return nil, fmt.Errorf("listing hosts: %w", err)
	}

	defer rows.Close()

	hosts := make([]model.Host, 0)

	for rows.Next() {
		var host model.Host

		if err := scanHost(rows, &host); err != nil {
			return nil, fmt.Errorf("scanning host row: %w", err)
		}

		hosts = append(hosts, host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading host rows: %w", err)
	}

	return hosts, nil
}

func (s *Store) GetHostByID(ctx context.Context, id int64) (*model.Host, error) {
	var host model.Host

	err := scanHost(s.db.QueryRowContext(ctx, hostSelect+`WHERE h.id = ?`, id), &host)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("getting host %d: %w", id, err)
	}

	return &host, nil
}

func (s *Store) HostIDByTokenHash(ctx context.Context, tokenHash string) (int64, bool, error) {
	var id int64

	err := s.db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE token_hash = ?`, tokenHash).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("looking up host by token: %w", err)
	}

	return id, true, nil
}

func (s *Store) UpdateHostToken(ctx context.Context, id int64, tokenHash string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE hosts SET token_hash = ? WHERE id = ?`, tokenHash, id)

	if err != nil {
		return false, fmt.Errorf("updating host %d token: %w", id, err)
	}

	updated, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking updated host %d: %w", id, err)
	}

	return updated != 0, nil
}

func (s *Store) DeleteHost(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)

	if err != nil {
		return false, fmt.Errorf("deleting host %d: %w", id, err)
	}

	deleted, err := result.RowsAffected()

	if err != nil {
		return false, fmt.Errorf("checking deleted host %d: %w", id, err)
	}

	return deleted != 0, nil
}

func (s *Store) TouchHost(ctx context.Context, id int64, agentVersion string, protocolVersion int, seenAt *time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE hosts SET last_seen_at = COALESCE(?, last_seen_at), agent_version = ?, protocol_version = ? WHERE id = ?
	`, seenAt, agentVersion, protocolVersion, id)

	if err != nil {
		return fmt.Errorf("recording host %d check-in: %w", id, err)
	}

	return nil
}

func (s *Store) RecordHostDockerError(ctx context.Context, id int64, snapshot model.HostSnapshot, seenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE hosts SET last_seen_at = ?, agent_version = ?, protocol_version = ?, docker_error = ?, fingerprint = ? WHERE id = ?
	`, seenAt, snapshot.AgentVersion, snapshot.ProtocolVersion, snapshot.DockerError, snapshot.Fingerprint, id)

	if err != nil {
		return fmt.Errorf("recording host %d docker error: %w", id, err)
	}

	return nil
}

func (s *Store) ReplaceHostSnapshot(ctx context.Context, id int64, snapshot model.HostSnapshot, seenAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)

	if err != nil {
		return fmt.Errorf("starting host snapshot transaction: %w", err)
	}

	defer tx.Rollback()

	info := snapshot.Info

	result, err := tx.ExecContext(ctx, `
		UPDATE hosts
		SET last_seen_at = ?, agent_version = ?, protocol_version = ?, hostname = ?, docker_version = ?,
			os = ?, architecture = ?, cpus = ?, memory_bytes = ?, docker_error = ?, fingerprint = ?
		WHERE id = ?
	`, seenAt, snapshot.AgentVersion, snapshot.ProtocolVersion, info.Hostname, info.DockerVersion,
		info.OS, info.Architecture, info.CPUs, info.MemoryBytes, snapshot.DockerError, snapshot.Fingerprint, id)

	if err != nil {
		return fmt.Errorf("updating host %d: %w", id, err)
	}

	if updated, err := result.RowsAffected(); err != nil || updated == 0 {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM host_containers WHERE host_id = ?`, id); err != nil {
		return fmt.Errorf("clearing host %d containers: %w", id, err)
	}

	insert, err := tx.PrepareContext(ctx, `
		INSERT INTO host_containers (
			host_id, container_id, name, image, image_host, image_repository, image_tag, image_digest,
			running_digests, ref_digests, state, health, exit_code, compose_project, compose_service,
			created_at, started_at, finished_at, ports
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)

	if err != nil {
		return fmt.Errorf("preparing host container insert: %w", err)
	}

	defer insert.Close()

	for _, c := range snapshot.Containers {
		_, err := insert.ExecContext(ctx,
			id, c.ContainerID, c.Name, c.Image, c.ImageHost, c.ImageRepository, c.ImageTag, c.ImageDigest,
			marshalList(c.RunningDigests), marshalList(c.RefDigests), c.State, c.Health, c.ExitCode,
			c.ComposeProject, c.ComposeService, c.CreatedAt, c.StartedAt, c.FinishedAt, marshalList(c.Ports),
		)

		if err != nil {
			return fmt.Errorf("inserting container %q for host %d: %w", c.Name, id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing host %d snapshot: %w", id, err)
	}

	return nil
}

func (s *Store) ListHostContainers(ctx context.Context, hostID int64) ([]model.HostContainer, error) {
	return s.listHostContainers(ctx, `WHERE host_id = ? ORDER BY compose_project IS NULL, compose_project, name`, hostID)
}

func (s *Store) ListAllHostContainers(ctx context.Context) ([]model.HostContainer, error) {
	return s.listHostContainers(ctx, `ORDER BY host_id, name`)
}

func (s *Store) ListContainersByRepository(ctx context.Context, repository string) ([]model.HostContainer, error) {
	return s.listHostContainers(ctx, `WHERE image_repository = ? ORDER BY host_id, name`, repository)
}

func (s *Store) listHostContainers(ctx context.Context, where string, args ...any) ([]model.HostContainer, error) {
	rows, err := s.db.QueryContext(ctx, hostContainerSelect+where, args...)

	if err != nil {
		return nil, fmt.Errorf("listing host containers: %w", err)
	}

	defer rows.Close()

	containers := make([]model.HostContainer, 0)

	for rows.Next() {
		var container model.HostContainer

		if err := scanHostContainer(rows, &container); err != nil {
			return nil, fmt.Errorf("scanning host container row: %w", err)
		}

		containers = append(containers, container)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading host container rows: %w", err)
	}

	return containers, nil
}
