CREATE TABLE worker_ticks (
    job         TEXT NOT NULL PRIMARY KEY,
    last_run_at DATETIME NOT NULL
);
