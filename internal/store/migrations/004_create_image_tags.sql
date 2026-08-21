CREATE TABLE image_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    image_id INTEGER NOT NULL,
    family_id INTEGER NOT NULL,
    family_type TEXT NOT NULL,
    tag TEXT NOT NULL,
    tag_order INTEGER NOT NULL,
    first_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    new BOOLEAN NOT NULL DEFAULT 1,

    UNIQUE(image_id, tag),
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE
);

CREATE INDEX idx_image_tags_image_id
    ON image_tags(image_id);