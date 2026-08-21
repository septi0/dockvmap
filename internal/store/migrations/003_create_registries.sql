CREATE TABLE registries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    registry TEXT NOT NULL UNIQUE,
    username TEXT,
    credential_nonce BLOB,
    credential_ciphertext BLOB,
    options TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

INSERT INTO registries (registry, created_at, updated_at) VALUES
    ('docker.io', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('ghcr.io', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);