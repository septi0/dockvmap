CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    data TEXT,
    ip TEXT,
    user_agent TEXT,
    user_id INTEGER,
    username TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_logs_created_at
    ON audit_logs(created_at DESC, id DESC);
