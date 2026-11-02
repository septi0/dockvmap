DROP TABLE host_containers;
DROP TABLE hosts;

DELETE FROM audit_logs WHERE type = 'HOST_TOKEN_REGENERATED';

CREATE TABLE hosts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL UNIQUE,
    agent_fingerprint TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME,
    poll_error TEXT,
    poll_error_kind TEXT,
    agent_version TEXT,
    protocol_version INTEGER,
    hostname TEXT,
    docker_version TEXT,
    os TEXT,
    architecture TEXT,
    cpus INTEGER,
    memory_bytes INTEGER,
    docker_error TEXT,
    report_fingerprint TEXT
);

CREATE TABLE host_containers (
    host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    image_host TEXT NOT NULL,
    image_repository TEXT NOT NULL,
    image_tag TEXT,
    image_digest TEXT,
    running_digests TEXT NOT NULL DEFAULT '[]',
    ref_digests TEXT NOT NULL DEFAULT '[]',
    state TEXT NOT NULL,
    health TEXT,
    exit_code INTEGER NOT NULL DEFAULT 0,
    compose_project TEXT,
    compose_service TEXT,
    created_at DATETIME,
    started_at DATETIME,
    finished_at DATETIME,
    ports TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (host_id, container_id)
);

CREATE INDEX idx_host_containers_image ON host_containers(image_repository);
