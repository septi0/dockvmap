CREATE TABLE tags_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    image_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    data TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    notify BOOLEAN NOT NULL DEFAULT 0,
    notif_sent_at DATETIME,
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE
);

CREATE INDEX idx_tags_events_created_at
    ON tags_events(created_at DESC, id DESC);

CREATE INDEX idx_tags_events_image_id_created_at
    ON tags_events(image_id, created_at DESC);

CREATE INDEX idx_tags_events_notif_pending
    ON tags_events(notif_sent_at)
    WHERE notif_sent_at IS NULL;