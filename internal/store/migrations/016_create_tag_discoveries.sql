CREATE TABLE tag_discoveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    registry_id INTEGER NOT NULL,
    repository TEXT NOT NULL,
    status TEXT NOT NULL,
    result_json TEXT,
    tag_count INTEGER,
    raw_tag_count INTEGER,
    error TEXT,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,

    UNIQUE(registry_id, repository),
    FOREIGN KEY (registry_id) REFERENCES registries(id) ON DELETE CASCADE
);
