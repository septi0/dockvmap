CREATE TABLE cached_blobs (
    digest TEXT PRIMARY KEY,
    size INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    accessed_at DATETIME NOT NULL
);

CREATE INDEX cached_blobs_accessed_at_idx ON cached_blobs(accessed_at);