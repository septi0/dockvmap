CREATE TABLE background_failures (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL,
    occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_background_failures_occurred_at ON background_failures (occurred_at DESC);
