CREATE TABLE replication_followers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    follower_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL,
    token_id INTEGER REFERENCES replication_tokens(id) ON DELETE SET NULL,
    version TEXT NOT NULL DEFAULT '',
    first_seen_at DATETIME NOT NULL,
    last_seen_at DATETIME NOT NULL
);

ALTER TABLE follower_state ADD COLUMN follower_id TEXT NOT NULL DEFAULT '';
