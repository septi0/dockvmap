CREATE TABLE proxy_metrics_daily (
    day                  TEXT NOT NULL PRIMARY KEY,
    total_requests       INTEGER NOT NULL DEFAULT 0,
    manifest_requests    INTEGER NOT NULL DEFAULT 0,
    blob_requests        INTEGER NOT NULL DEFAULT 0,
    cache_hits           INTEGER NOT NULL DEFAULT 0,
    cache_misses         INTEGER NOT NULL DEFAULT 0,
    upstream_requests    INTEGER NOT NULL DEFAULT 0,
    upstream_failures    INTEGER NOT NULL DEFAULT 0,
    cache_write_failures INTEGER NOT NULL DEFAULT 0
);
