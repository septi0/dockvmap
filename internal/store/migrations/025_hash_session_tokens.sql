DELETE FROM sessions;

DROP INDEX idx_sessions_token;

ALTER TABLE sessions RENAME COLUMN token TO token_hash;

CREATE UNIQUE INDEX idx_sessions_token_hash ON sessions(token_hash);
