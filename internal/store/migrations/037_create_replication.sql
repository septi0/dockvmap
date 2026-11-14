CREATE TABLE replication_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    cluster_id TEXT,
    version INTEGER NOT NULL DEFAULT 0,
    tag TEXT NOT NULL DEFAULT ''
);

INSERT INTO replication_state (id) VALUES (1);

CREATE TABLE replication_repositories (
    repository TEXT PRIMARY KEY,
    counter INTEGER NOT NULL
);

CREATE TABLE replication_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repository TEXT NOT NULL,
    reference TEXT NOT NULL,
    deleted INTEGER NOT NULL DEFAULT 0,
    counter INTEGER NOT NULL,
    changed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_replication_changes_repository ON replication_changes(repository, counter);
CREATE INDEX idx_replication_changes_changed_at ON replication_changes(changed_at);

CREATE TABLE replication_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    label TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME
);

CREATE TABLE follower_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    cluster_id TEXT,
    applied_version INTEGER NOT NULL DEFAULT 0,
    applied_tag TEXT NOT NULL DEFAULT '',
    applied_at DATETIME,
    initial_copy_done INTEGER NOT NULL DEFAULT 0,
    resync_requested INTEGER NOT NULL DEFAULT 0
);

INSERT INTO follower_state (id) VALUES (1);

CREATE TABLE follower_repositories (
    repository TEXT PRIMARY KEY,
    counter INTEGER NOT NULL
);

CREATE TRIGGER registries_replication_insert AFTER INSERT ON registries BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER registries_replication_update AFTER UPDATE OF registry, username, credential_ciphertext, options, internal ON registries
WHEN OLD.registry IS NOT NEW.registry OR OLD.username IS NOT NEW.username OR OLD.credential_ciphertext IS NOT NEW.credential_ciphertext
    OR OLD.options IS NOT NEW.options OR OLD.internal IS NOT NEW.internal
BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER registries_replication_delete AFTER DELETE ON registries BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER image_groups_replication_insert AFTER INSERT ON image_groups BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER image_groups_replication_update AFTER UPDATE OF name ON image_groups
WHEN OLD.name IS NOT NEW.name
BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER image_groups_replication_delete AFTER DELETE ON image_groups BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER images_replication_insert AFTER INSERT ON images BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER images_replication_update AFTER UPDATE OF name, registry_id, repository, tag, pinned, group_id ON images
WHEN OLD.name IS NOT NEW.name OR OLD.registry_id IS NOT NEW.registry_id OR OLD.repository IS NOT NEW.repository
    OR OLD.tag IS NOT NEW.tag OR OLD.pinned IS NOT NEW.pinned OR OLD.group_id IS NOT NEW.group_id
BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER images_replication_delete AFTER DELETE ON images BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER proxy_tokens_replication_insert AFTER INSERT ON proxy_tokens BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER proxy_tokens_replication_update AFTER UPDATE OF label, token_hash, can_push ON proxy_tokens
WHEN OLD.label IS NOT NEW.label OR OLD.token_hash IS NOT NEW.token_hash OR OLD.can_push IS NOT NEW.can_push
BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER proxy_tokens_replication_delete AFTER DELETE ON proxy_tokens BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER replication_repositories_replication_insert AFTER INSERT ON replication_repositories BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER replication_repositories_replication_update AFTER UPDATE OF counter ON replication_repositories
WHEN OLD.counter IS NOT NEW.counter
BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;

CREATE TRIGGER replication_repositories_replication_delete AFTER DELETE ON replication_repositories BEGIN
    UPDATE replication_state SET version = version + 1, tag = lower(hex(randomblob(8))) WHERE id = 1;
END;
